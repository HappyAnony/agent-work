// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/88250/lute/ast"
	"github.com/siyuan-note/logging"

	"wiki-kernel/cache"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/notebookindex"
	"wiki-kernel/internal/pageprefs"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/sql"
	"wiki-kernel/task"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func (r *indexRepairContext) indexMarkdown(ctx context.Context) (out nativeparser.Markdown, err error) {
	if e := r.guard(ctx); e != nil {
		return out, e
	}
	err = r.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.Editor == nil || v.Editor.Markdown == nil {
			return conf.ErrBusinessUnbound
		}
		out = *v.Editor.Markdown
		return r.guard(ctx)
	})
	return
}

func (r *indexRepairContext) indexNotebook(ctx context.Context, id string) (repairIOErr error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	boxes, e := r.owner.notebooks(ctx)
	if e != nil {
		return e
	}
	opened := false
	name := ""
	for _, b := range boxes {
		if b.ID == id && !b.Closed {
			opened = true
			name = b.Name
			break
		}
	}
	if !opened {
		return nil
	}
	markdown, e := r.indexMarkdown(ctx)
	if e != nil {
		return e
	}
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, true)
	if e != nil {
		return e
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	if e = r.setRepairBootDetails(ctx, "Listing files..."); e != nil {
		return e
	}
	docs, e := ioctx.Source.Documents(ctx, id)
	if e != nil {
		return e
	}
	if e = r.queue.Flush(ctx); e != nil {
		return e
	}
	if e = r.pushRepairStatus(ctx, fmt.Sprintf("[%s] "+r.language(64), util.EscapeHTML(name), len(docs))); e != nil {
		return e
	}
	return notebookindex.Rebuild(ctx, docs, notebookindex.Services{Workers: 4,
		Process: func(ctx context.Context, p indexrepair.DocumentPath) ([]*ast.Node, error) {
			engine := nativeparser.NewLute(markdown)
			tree, e := ioctx.Load(ctx, p.Box, p.Path, engine, true)
			if e != nil {
				return nil, e
			}
			if tree.Root.IALAttr("updated") == "" {
				tree.Root.SetIALAttr("updated", util.TimeFromID(tree.ID))
				if e = r.writeRepairTree(ctx, ioctx, tree, engine); e != nil {
					return nil, e
				}
			}
			if e = r.guard(ctx); e != nil {
				return nil, e
			}
			critical := context.WithoutCancel(ctx)
			if e = r.registry.Upsert(critical, tree); e != nil {
				return nil, e
			}
			// Original full projection, using difference/upsert instead of duplicate
			// insert when a notebook initialization overlaps an already indexed tree.
			if e = r.queue.Upsert(critical, tree); e != nil {
				return nil, e
			}
			if e = r.invalidateIncremental(critical, tree.ID, tree.Path); e != nil {
				return nil, e
			}
			part := int32(0)
			if len(docs) > 0 {
				part = int32(30.0 / float64(len(boxes)) / float64(len(docs)))
			}
			if e = r.incRepairBootProgress(ctx, part, fmt.Sprintf(r.language(92), util.ShortPathForBootingDisplay(tree.Path))); e != nil {
				return nil, e
			}
			return tree.Root.ChildrenByType(ast.NodeAttributeView), nil
		},
		Flush: func(ctx context.Context) error {
			if e := r.guard(ctx); e != nil {
				return e
			}
			return r.queue.Flush(ctx)
		},
		Bind: func(ctx context.Context, nodes []*ast.Node) error {
			return r.bindRepairAVBatch(ctx, ioctx.Source, nodes)
		},
		History: func(ctx context.Context) error {
			if e := r.guard(ctx); e != nil {
				return e
			}
			if e := closeIO(); e != nil {
				return e
			}
			r.owner.historyGenerated.Store(id, time.Now())
			return nil
		},
	})
}
func (r *indexRepairContext) indexReferences(ctx context.Context) (err error) {
	defer recoverRepositoryWorkflow(&err, "notebook_references")
	if e := r.guard(ctx); e != nil {
		return e
	}
	markdown, e := r.indexMarkdown(ctx)
	if e != nil {
		return e
	}
	ioctx, closeIO, e := r.openRepairIOChecked(ctx, false)
	if e != nil {
		return e
	}
	defer func() { err = joinRepairCloseError(err, closeIO()) }()
	queue := r.queue
	defer func() { err = errors.Join(err, indexrepairDrain(ctx, queue)) }()
	if e = queue.Flush(ctx); e != nil {
		return e
	}
	boxes, e := r.owner.notebooks(ctx)
	if e != nil {
		return e
	}
	if e = r.setRepairBootDetails(ctx, "Indexing refs..."); e != nil {
		return e
	}
	if e = r.pushRepairStatus(ctx, r.language(54)); e != nil {
		return e
	}
	count := 0
	for _, b := range boxes {
		if b.Closed {
			continue
		}
		docs, e := ioctx.Source.Documents(ctx, b.ID)
		if e != nil {
			return e
		}
		for _, p := range docs {
			data, e := ioctx.Source.Read(ctx, p.Relative)
			if e != nil {
				return e
			}
			if !bytes.Contains(data, []byte("TextMarkBlockRefID")) && !bytes.Contains(data, []byte("TextMarkFileAnnotationRefID")) {
				continue
			}
			rows, e := r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: p.ID})
			if e != nil {
				return e
			}
			if len(rows) == 0 || rows[0].BoxID != b.ID || rows[0].Path != p.Path {
				return indexrepair.ErrReferenceTree
			}
			tree, e := ioctx.Load(ctx, b.ID, p.Path, nativeparser.NewLute(markdown), false)
			if e != nil {
				return e
			}
			has := false
			ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
				if entering && (treenode.IsBlockRef(n) || treenode.IsFileAnnotationRef(n)) {
					has = true
					return ast.WalkStop
				}
				return ast.WalkContinue
			})
			if !has {
				continue
			}
			if e = r.guard(ctx); e != nil {
				return e
			}
			if e = r.queue.UpdateRefs(ctx, tree); e != nil {
				return e
			}
			count++
		}
	}
	if e = queue.Flush(ctx); e != nil {
		return e
	}
	if e = closeIO(); e != nil {
		return e
	}
	return r.pushRepairStatus(ctx, fmt.Sprintf(r.language(55), count))
}
func indexrepairDrain(ctx context.Context, q sql.LegacyMainQueue) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("notebook_drain_failed")
		}
	}()
	return q.Flush(context.WithoutCancel(ctx))
}
func (r *indexRepairContext) unindexNotebook(ctx context.Context, id string) (repairIOErr error) {
	if e := r.guard(ctx); e != nil {
		return e
	}
	if !ast.IsNodeIDPattern(id) {
		return indexrepair.ErrSourcePath
	}
	if r.main.DB == nil && r.registry.DB == nil {
		return ctx.Err()
	}
	if e := r.queue.Flush(ctx); e != nil {
		return e
	}
	critical := context.WithoutCancel(ctx)
	rows, e := r.registry.Query(critical, indexdb.BlockTreeFilter{Kind: "box", Value: id})
	if e != nil {
		return e
	}
	for _, b := range rows {
		cache.InvalidateTreeIAL(b.Path, []string{b.ID})
	}
	if _, e = r.registry.Remove(critical, indexdb.BlockTreeFilter{Kind: "box", Value: id}); e != nil {
		return e
	}
	ioctx, closeIO, e := r.openRepairIOChecked(critical, true)
	if e != nil {
		return e
	}
	defer func() { repairIOErr = joinRepairCloseError(repairIOErr, closeIO()) }()
	if e = r.removeNotebookRecent(critical, ioctx.Source); e != nil {
		return e
	}
	if e = r.queue.DeleteBox(critical, id); e != nil {
		return e
	}
	return r.queue.Flush(critical)
}
func (c *AppConf) executeNotebookIndex(ctx context.Context, request cloudrepo.NotebookIndexRequest) error {
	if request.Kind == "full" {
		r, e := captureIndexRepair(ctx)
		if e != nil {
			return e
		}
		if e = r.guard(ctx); e != nil {
			return e
		}
		if e := reindexAfterCheckout(ctx); e != nil {
			return e
		}
		c.publishSyncUI(ctx, util.ReloadUI)
		return ctx.Err()
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return e
	}
	switch request.Kind {
	case "index":
		if e = r.indexNotebook(ctx, request.ID); e != nil {
			return e
		}
		if e = r.indexReferences(ctx); e != nil {
			return e
		}
	case "unindex":
		if e = r.unindexNotebook(ctx, request.ID); e != nil {
			return e
		}
	case "refs":
		return r.indexReferences(ctx)
	case "init":
		count, e := r.registry.Count(ctx, indexdb.BlockTreeFilter{Kind: "all"})
		if e != nil {
			return e
		}
		boxes, e := c.notebooks(ctx)
		if e != nil {
			return e
		}
		for _, b := range boxes {
			if b.Closed {
				continue
			}
			if count == 0 {
				if e = r.indexNotebook(ctx, b.ID); e != nil {
					return e
				}
			} else {
				c.historyGenerated.Store(b.ID, time.Now())
			}
		}
	}
	if e = r.guard(ctx); e != nil {
		return e
	}
	return ResetVirtualBlockRefCacheContext(ctx)
}
func NotebookIndexContext(ctx context.Context, kind, id string) error {
	s, e := Conf.CloudRepoService()
	if e != nil {
		return e
	}
	return s.NotebookIndex(ctx, cloudrepo.NotebookIndexRequest{Kind: kind, ID: id})
}
func InitBoxesContext(ctx context.Context) error { return NotebookIndexContext(ctx, "init", "") }
func (c *AppConf) scheduleFullNotebookReindex() bool {
	return c.scheduleFullNotebookReindexContext(context.Background()) == nil
}

func (c *AppConf) scheduleFullNotebookReindexContext(ctx context.Context) error {
	return c.admitFullNotebookReindex(ctx, nil, nil)
}

// Admission and its completion effects share the owner's shutdown gate. The
// task may replace query databases, so authorization runs before it starts.
func (c *AppConf) admitFullNotebookReindex(ctx context.Context, authorize func() error, admitted func()) error {
	s, e := c.CloudRepoService()
	if e != nil {
		return e
	}
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if authorize != nil {
		if err := authorize(); err != nil {
			return err
		}
	}
	if c.repoClosed {
		return errors.New("full_notebook_reindex_owner_closed")
	}
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	if !c.repoTasks.Go(func(ctx context.Context) {
		h, e := task.AppendContext(ctx, task.DatabaseIndexFull, func(work context.Context) error {
			return s.NotebookIndex(work, cloudrepo.NotebookIndexRequest{Kind: "full"})
		})
		if e == nil {
			e = h.Wait(context.WithoutCancel(ctx))
		}
		if e != nil && ctx.Err() == nil {
			logging.LogErrorf("full notebook rebuild failed: %s", e)
			c.notifyRepo(ctx, e.Error(), 5000)
		}
	}) {
		return errors.New("full_notebook_reindex_not_admitted")
	}
	if admitted != nil {
		admitted()
	}
	return nil
}

func (r *indexRepairContext) removeNotebookRecent(ctx context.Context, source indexrepair.Source) error {
	const name = "storage/recent-doc.json"
	var before []byte
	backend := pageprefs.Backend{Now: time.Now, RefreshRecent: true,
		Read: func(c context.Context, _ string) ([]byte, error) {
			var e error
			before, e = source.Read(c, name)
			if indexrepair.IsSourceNotExist(e) {
				before = nil
				return nil, fs.ErrNotExist
			}
			return before, e
		},
		Write: func(c context.Context, _ string, raw []byte) error { return source.Rewrite(c, name, before, raw) },
		Paths: func(c context.Context, ids []string) (map[string]string, error) {
			rows, e := r.registry.Query(c, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
			if e != nil {
				return nil, e
			}
			out := map[string]string{}
			for _, v := range rows {
				out[v.ID] = v.HPath
			}
			return out, nil
		},
	}
	_, e := r.owner.preferencesService().Do(ctx, pageprefs.Request{Action: "recent.compact", SortBy: "viewedAt"}, backend)
	return e
}
