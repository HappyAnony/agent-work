// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"path"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/pageprefs"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func runStructureConversion(ctx context.Context, q doclife.StructureRequest) (result doclife.StructurePlan, err error) {
	if ctx == nil {
		return result, errors.New("structural_document_context_unbound")
	}
	o, e := captureLegacyDocumentLifecycle(ctx)
	if e != nil {
		return result, e
	}
	_, err = o.run(ctx, func(op, life context.Context) (doclife.Plan, error) {
		if e := FlushTxQueueContext(op); e != nil {
			return doclife.Plan{}, e
		}
		env := legacyTransactionEnvironmentContext(op)
		env.ctx = op
		env.parser = o.parser
		if e := bindLegacyCommit(op, env, o.r); e != nil {
			return doclife.Plan{}, e
		}
		base := o.services(life)
		historyTrees := map[string]*parse.Tree{}
		load := base.Load
		base.Load = func(c context.Context, box, p string) (*parse.Tree, error) {
			t, e := load(c, box, p)
			if e != nil {
				return nil, e
			}
			pre := &parse.Tree{Box: t.Box, Path: t.Path, ID: t.ID, Root: &ast.Node{Type: ast.NodeDocument, ID: t.ID}}
			for _, n := range t.Root.ChildrenByType(ast.NodeAttributeView) {
				pre.Root.AppendChild(&ast.Node{Type: ast.NodeAttributeView, AttributeViewID: n.AttributeViewID})
			}
			historyTrees[box+p] = pre
			return t, nil
		}
		services := doclife.StructureServices{Lifecycle: base, Body: doclife.TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now},
			ByPath: func(c context.Context, box, p string) (doclife.Location, bool, error) {
				if e := o.guard(c); e != nil {
					return doclife.Location{}, false, e
				}
				rows, e := o.r.registry.Query(c, indexdb.BlockTreeFilter{Kind: "root-path", BoxID: box, Value: p})
				if e != nil {
					return doclife.Location{}, false, e
				}
				if len(rows) == 0 {
					return doclife.Location{}, false, nil
				}
				v := rows[0]
				return doclife.Location{Box: v.BoxID, Path: v.Path, HPath: v.HPath, ID: v.ID, Document: true}, true, nil
			},
			Text: func(c context.Context, n *ast.Node) (string, error) {
				return doclife.HeadingTitle(c, n, env.commitContext.anchorMax, indexdb.ContentServices{AttributeView: func(c context.Context, id string, full bool) (string, error) {
					return env.commitContext.avContext(c).title(id)
				}})
			},
			CanRemove: func(c context.Context, loc doclife.Location) (err error) {
				io, closeIO, e := o.r.openRepairIOChecked(c, false)
				if e != nil {
					return e
				}
				defer func() { err = errors.Join(err, closeIO()) }()
				entries, e := io.Source.ReadDir(c, loc.Box+strings.TrimSuffix(loc.Path, ".docj"))
				if indexrepair.IsSourceNotExist(e) {
					return nil
				}
				if e != nil {
					return e
				}
				if len(entries) > 0 {
					return errors.New(o.language(20))
				}
				return nil
			},
		}
		var e error
		result, e = doclife.PrepareStructure(op, q, services)
		if e != nil {
			return doclife.Plan{}, e
		}
		if result.DeleteSource {
			h := &historyIndexContext{owner: o.owner, root: o.history, workspace: o.r.workspace, queue: sql.CaptureLegacyHistoryQueue(), untitled: o.plain(16)}
			if e = writeTransactionHistory(op, env.commitContext, h, historyTrees[result.Source.Box+result.Source.Path], time.Now(), func(name string) error { return h.index(op, name, o.parser()) }); e != nil {
				return doclife.Plan{}, e
			}
		}
		return doclife.Plan{}, o.applyStructure(op, life, env.commitContext, q, result)
	})
	if err != nil {
		return doclife.StructurePlan{}, err
	}
	return
}

// Sequential compatibility publication retains its original successful-prefix
// contract. Every source/index/sort/notification error reaches the caller; this
// does not claim the journaled cross-file transaction supplied by R04.
func (o *legacyDocumentLifecycleContext) applyStructure(ctx, life context.Context, c *legacyCommitContext, q doclife.StructureRequest, p doclife.StructurePlan) (err error) {
	if err = o.guard(ctx); err != nil {
		return err
	}
	io, closeIO, err := o.r.openRepairIOChecked(ctx, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	checkRevision := func(t *parse.Tree) error {
		v, ok := o.r.revisions.Load(t.Box + t.Path)
		if !ok {
			return indexrepair.ErrSourceChanged
		}
		raw, e := io.Source.Read(ctx, t.Box+t.Path)
		if e != nil {
			return e
		}
		if sha256.Sum256(raw) != v.([32]byte) {
			return indexrepair.ErrSourceChanged
		}
		return nil
	}
	if err = checkRevision(p.Source); err != nil {
		return err
	}
	if p.DeleteSource {
		if err = checkRevision(p.Target); err != nil {
			return err
		}
	}
	if err = o.guard(ctx); err != nil {
		return err
	}
	critical := context.WithoutCancel(ctx)
	// Once source publication is accepted, complete the captured derived queue
	// before releasing this owner, including on a later publication failure.
	admitted := false
	defer func() {
		if admitted {
			err = errors.Join(err, o.r.queue.Flush(critical))
		}
	}()
	if p.DeleteSource {
		folder := p.Source.Box + strings.TrimSuffix(p.Source.Path, ".docj")
		entries, e := io.Source.ReadDir(ctx, folder)
		if e == nil {
			if len(entries) > 0 {
				return errors.New("structural_document_has_children")
			}
			if e = io.Source.Remove(ctx, folder); e != nil {
				return e
			}
		} else if !indexrepair.IsSourceNotExist(e) {
			return e
		}
		if err = io.Source.Remove(ctx, p.Source.Box+p.Source.Path); err != nil {
			return err
		}
		if _, err = o.r.registry.Remove(critical, indexdb.BlockTreeFilter{Kind: "root", Value: p.Source.ID}); err != nil {
			return err
		}
		if err = o.r.queue.Remove(critical, p.Source.ID); err != nil {
			return err
		}
		admitted = true
		if err = o.removeStructureSort(critical, io.Source, p.Source.Box, p.Source.ID); err != nil {
			return err
		}
		if _, err = StorageContext(critical, pageprefs.Request{Action: "recent.remove", Keys: []string{p.Source.ID}}); err != nil {
			return err
		}
	} else {
		if _, err = o.r.registry.Remove(critical, indexdb.BlockTreeFilter{Kind: "root", Value: p.Source.ID}); err != nil {
			return err
		}
		if err = (&legacyAnchorContext{r: o.r, commit: c}).stage(critical, p.Source, p.Source); err != nil {
			return err
		}
		if err = o.insertStructureSort(critical, io.Source, p.Target, q.PreviousPath); err != nil {
			return err
		}
	}
	if p.DeleteSource {
		if _, err = o.r.registry.Remove(critical, indexdb.BlockTreeFilter{Kind: "root", Value: p.Target.ID}); err != nil {
			return err
		}
		if err = (&legacyAnchorContext{r: o.r, commit: c}).stage(critical, p.Target, p.Target); err != nil {
			return err
		}
	} else {
		if err = io.Write(critical, p.Target, nil, o.parser()); err != nil {
			return err
		}
		if err = o.r.registry.Upsert(critical, p.Target); err != nil {
			return err
		}
		if err = o.r.queue.Upsert(critical, p.Target); err != nil {
			return err
		}
		admitted = true
		if err = o.r.queue.UpdateRefs(critical, p.Target); err != nil {
			return err
		}
	}
	if err = o.r.queue.Flush(critical); err != nil {
		return err
	}
	admitted = false
	o.owner.syncRuntime.same.Store(0)
	o.owner.planSyncAfter(c.interval)
	owner, e := captureStructuralDocumentOwner(critical)
	if e != nil {
		return e
	}
	if e = owner.scheduleReferenceRefresh(p.Source.ID, p.Target.ID); e != nil {
		return e
	}
	if p.DeleteSource {
		evt := util.NewCmdResult("removeDoc", 0, util.PushModeBroadcast)
		evt.Data = map[string]interface{}{"ids": []string{p.Source.ID}}
		o.owner.publishSyncUI(life, func() { util.PushEvent(evt) })
	}
	for _, effect := range p.Effects {
		if effect == "heading-title-slash-replaced" {
			o.owner.publishSyncUI(life, func() { util.PushMsg(o.plain(246), 7000) })
		}
	}
	return ctx.Err()
}

func (o *legacyDocumentLifecycleContext) removeStructureSort(ctx context.Context, source indexrepair.Source, box, id string) error {
	name := box + "/.siyuan/sort.json"
	raw, e := source.Read(ctx, name)
	if indexrepair.IsSourceNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	values := map[string]int{}
	if e = json.Unmarshal(raw, &values); e != nil {
		return e
	}
	if values == nil {
		return errors.New("invalid_notebook_sort")
	}
	delete(values, id)
	out, e := json.Marshal(values)
	if e != nil {
		return e
	}
	return source.Rewrite(ctx, name, raw, out)
}
func (o *legacyDocumentLifecycleContext) insertStructureSort(ctx context.Context, source indexrepair.Source, t *parse.Tree, previous string) error {
	if previous != "" {
		return o.r.addConflictSort(ctx, t.Box, previous, t.ID)
	}
	var top bool
	if e := o.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.FileTree == nil || v.FileTree.CreateDocAtTop == nil {
			return conf.ErrBusinessUnbound
		}
		top = *v.FileTree.CreateDocAtTop
		return nil
	}); e != nil {
		return e
	}
	max := 102400
	if top {
		max = 1
	}
	docs, _, err := listDocTreeContext(ctx, t.Box, path.Dir(t.Path), util.SortModeUnassigned, false, false, max, false)
	if err != nil {
		return err
	}
	rank := 0
	if len(docs) > 0 {
		if top {
			rank = docs[0].Sort - 1
		} else {
			rank = docs[len(docs)-1].Sort + 1
		}
	}
	name := t.Box + "/.siyuan/sort.json"
	raw, err := source.Read(ctx, name)
	if indexrepair.IsSourceNotExist(err) {
		raw = nil
	} else if err != nil {
		return err
	}
	values := map[string]int{}
	if raw != nil {
		if err = json.Unmarshal(raw, &values); err != nil {
			return err
		}
		if values == nil {
			return errors.New("invalid_notebook_sort")
		}
	}
	values[t.ID] = rank
	out, err := json.Marshal(values)
	if err != nil {
		return err
	}
	if err = source.Rewrite(ctx, name, raw, out); err != nil {
		return err
	}
	notice := map[string]int{t.ID: rank}
	for _, doc := range docs {
		notice[doc.ID] = doc.Sort
	}
	o.owner.publishSyncUI(ctx, func() { pushFiletreeSortChanged(notice) })
	return nil
}

func Doc2HeadingContext(ctx context.Context, source, target string, after bool) (string, string, error) {
	p, e := runStructureConversion(ctx, doclife.StructureRequest{Action: "doc-to-heading", ID: source, TargetID: target, After: after})
	if e != nil {
		return "", "", e
	}
	return p.Source.Box, p.Source.Path, nil
}
func Heading2DocContext(ctx context.Context, id, box, target, previous string) (string, string, error) {
	p, e := runStructureConversion(ctx, doclife.StructureRequest{Action: "heading-to-doc", ID: id, TargetBox: box, TargetPath: target, PreviousPath: previous})
	if e != nil {
		return "", "", e
	}
	return p.Source.ID, p.Target.Path, nil
}
