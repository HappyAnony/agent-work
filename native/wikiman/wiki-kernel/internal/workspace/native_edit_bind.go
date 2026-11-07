// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute"
	"io/fs"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/vmihailenco/msgpack/v5"
	"wiki-kernel/internal/anchor"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avquery"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/internal/indexdb"
)

func (p *nativeEdit) bind() {
	noop := func(string) error { return p.ctx.Err() }
	cache := func(string, map[string]string) error { return p.ctx.Err() }
	attrsNotice := func(old map[string]string, n *ast.Node) error {
		return p.notice("attributes", n.ID, parse.IAL2Map(n.KramdownIAL))
	}

	p.cell = &editcore.AttributeViewCellEnvironment{Ctx: p.ctx, StrictDependencies: true, Read: p.readAV, Stage: p.saveAV, Groups: p.groups, Node: p.node, BlockText: func(n *ast.Node) (string, error) { _, text, e := p.text(n); return text, e }, Names: p.names, Attrs: p.attrs, Related: p.markRelated, Now: func() time.Time { return p.now }, NewID: ast.NewNodeID, Color: func() string { return strconv.Itoa(rand.IntN(14) + 1) }}
	p.defaults = &editcore.AttributeViewDefaultEnvironment{Ctx: p.ctx, Now: func() time.Time { return p.now }, NewID: ast.NewNodeID, Near: p.near, TemplateKeys: p.templateKeys, Reverse: func(a *av.AttributeView, k *av.Key, v *av.Value) error {
		return editcore.UpdateTwoWayRelationDestWithEnvironment(p.cell, a, k, v, 1, []string{})
	}}
	p.bound = &editcore.AttributeViewBoundEnvironment{Ctx: p.ctx, StrictDependencies: true, Read: p.readAV, Stage: p.saveAV, Groups: p.groups, Exists: p.exists, Load: func(id string) (*parse.Tree, error) { _, t, e := p.node(id); return t, e }, IsMirror: func(id string) (bool, error) { rows, e := p.mirrorMap(); return len(rows[id]) > 1, e }, RemoveMirror: p.removeMirror, Names: p.names, Attrs: p.attrs, Cache: cache, Notify: attrsNotice, Tree: p.stage, Reload: noop}
	p.tx.Environment = &editcore.DocumentEnvironment{
		Ctx: p.ctx, ReadGuard: p.ctx.Err, LookupBlock: p.lookup, Load: p.load, Stage: p.stage, CacheIAL: func(string, map[string]string) {}, ApplyHeadingEffects: p.stage,
		Now: func() string { return p.now.Format("20060102150405") }, EditorParser: func() *lute.Lute { return p.parser }, MoveHistory: func(context.Context, *parse.Tree) error { return p.ctx.Err() }, OutlineHistory: func(context.Context, *parse.Tree) error { return p.ctx.Err() },
		MoveReferences: func(context.Context, ...string) error { return p.ctx.Err() }, DeleteReferences: func(_ context.Context, ids ...string) error { return p.notice("references", "", slices.Clone(ids)) }, InsertReferences: func(_ context.Context, ids ...string) error { return p.notice("references", "", slices.Clone(ids)) }, UpdateReferences: func(_ context.Context, ids ...string) error { return p.notice("references", "", slices.Clone(ids)) },
		ReferenceCounts: func(ctx context.Context, ids []string) (map[string]int, error) {
			db, e := p.bundle.Database(indexdb.Main)
			if e != nil {
				return nil, e
			}
			return (indexdb.References{DB: db}).DefinitionCounts(ctx, ids)
		}, AvNames: func(_ context.Context, ids string) (string, error) { return p.names(ids) },
		DeleteBlockTree: func(context.Context, string) error { return p.ctx.Err() },
		DeleteAV: func(_ context.Context, n *ast.Node, t *parse.Tree, _ *editcore.DocumentTransaction) error {
			return editcore.DeleteAttributeViewBindings(n, t, p.bound)
		},
		InsertAssets: func(_ context.Context, t *parse.Tree, _ *editcore.DocumentTransaction) error {
			return p.validateAssets(t)
		},
		InsertAVRegistry: func(_ context.Context, n *ast.Node) error {
			return editcore.RegisterUpdatedAttributeViews(n, p.updateEnvironment())
		},
		InsertAVBind: func(_ context.Context, n *ast.Node, t *parse.Tree, _ *editcore.DocumentTransaction) error {
			return p.insertBindings(n, t)
		},
		UpdateCacheRef: func(context.Context, *parse.Tree, *ast.Node) error { return p.ctx.Err() }, UpdateDynamicText: func(c context.Context, _ string) (string, error) { return "", c.Err() },
		UpdateUnfold: func(_ context.Context, h, n *ast.Node) error { return p.notice("unfoldHeading", h.ID, n.ID) },
		UpdateEvent: func(_ context.Context, events []*editcore.DocumentTransaction) error {
			return p.notice("transactions", "", events)
		},
		UpdateAV: func(_ context.Context, n *ast.Node, t *parse.Tree, _ *editcore.DocumentTransaction) error {
			return editcore.UpdateAttributeViewWithEnvironment(n, t, p.updateEnvironment())
		},
	}
}

func (p *nativeEdit) exists(id string) (bool, error) { n, _, e := p.node(id); return n != nil, e }

func (p *nativeEdit) runtime(a *av.AttributeView) (nativeAVRuntime, error) {
	return p.r.newNativeAVRuntime(p.ctx, p.w, a, p.now, &nativeAVOverlay{Read: p.readAV, Trees: func(ids []string) (map[string]*parse.Tree, error) {
		out := map[string]*parse.Tree{}
		for _, id := range ids {
			n, t, e := p.node(id)
			if e != nil {
				return nil, e
			}
			if n == nil || t == nil {
				return nil, editcore.ErrBlockNotFound
			}
			out[id] = t
		}
		return out, p.ctx.Err()
	}})
}

func (p *nativeEdit) groups(a *av.AttributeView) error {
	for _, candidate := range p.avs {
		candidate.RenderedViewables = map[string]av.Viewable{}
	}
	runtime, e := p.runtime(a)
	if e != nil {
		return e
	}
	for _, v := range a.Views {
		if v.GetGroupKey(a) == nil {
			continue
		}
		if e = avrender.GenerateGroups(v, a, avrender.GroupEnvironment{Now: func() time.Time { return p.now }, Read: p.readAV, Render: runtime.render, NewView: avrender.NewGroupView}); e != nil {
			return e
		}
	}
	return p.ctx.Err()
}

func (p *nativeEdit) view(a *av.AttributeView, id string) (*av.View, error) {
	view := ""
	if id != "" {
		n, _, e := p.node(id)
		if e != nil {
			return nil, e
		}
		if n != nil {
			view = n.IALAttr(av.NodeAttrView)
		}
	}
	return a.GetCurrentView(view)
}

func (p *nativeEdit) near(a *av.AttributeView, v, g *av.View, previous string) (av.Item, error) {
	runtime, e := p.runtime(a)
	if e != nil {
		return nil, e
	}
	if e = avrender.PrepareGroup(v, g); e != nil {
		return nil, e
	}
	out, e := runtime.render(a, g, "")
	if e != nil {
		return nil, e
	}
	further, e := avrender.FurtherCollections(p.ctx, a, runtime.cached, avrender.FurtherEnvironment{Read: runtime.read, Render: runtime.render})
	if e != nil {
		return nil, e
	}
	if e = av.FilterChecked(out, a, further, runtime.cached); e != nil {
		return nil, e
	}
	av.Sort(out, a)
	return editcore.SelectAttributeViewNearItem(out.(av.Collection), previous), p.ctx.Err()
}

func (p *nativeEdit) count(a *av.AttributeView, v, g *av.View) (int, error) {
	runtime, e := p.runtime(a)
	if e != nil {
		return 0, e
	}
	if e = avrender.PrepareGroup(v, g); e != nil {
		return 0, e
	}
	out, e := runtime.render(a, g, "")
	if e != nil {
		return 0, e
	}
	further, e := avrender.FurtherCollections(p.ctx, a, runtime.cached, avrender.FurtherEnvironment{Read: runtime.read, Render: runtime.render})
	if e != nil {
		return 0, e
	}
	if e = av.FilterChecked(out, a, further, runtime.cached); e != nil {
		return 0, e
	}
	av.Sort(out, a)
	av.Calc(out, a)
	return out.(av.Collection).CountItems(), p.ctx.Err()
}

func (p *nativeEdit) templateKeys(a *av.AttributeView, key *av.Key) ([]*av.Key, error) {
	db, e := p.bundle.Database(indexdb.Main)
	if e != nil {
		return nil, e
	}
	env := avrender.TemplateEnvironment{Context: p.ctx, Now: func() time.Time { return p.now }, Name: func(a *av.AttributeView) string { return a.Name }, Funcs: avrender.BuiltInTemplateFuncs(func() time.Time { return p.now }, p.indexer, indexdb.TemplateFunctions(p.ctx, db))}
	return avrender.GetTemplateKeyRelevantKeys(a, key, env), p.ctx.Err()
}

func (p *nativeEdit) mirrorMap() (map[string][]string, error) {
	raw, e := p.readFile("storage/av/blocks.msgpack")
	if errors.Is(e, fs.ErrNotExist) {
		return map[string][]string{}, nil
	}
	if e != nil {
		return nil, e
	}
	var rows map[string][]string
	if e = msgpack.Unmarshal(raw, &rows); e != nil {
		return nil, e
	}
	if rows == nil {
		return nil, errors.New("invalid_AV_mirror_registry")
	}
	return rows, p.ctx.Err()
}

func (p *nativeEdit) mirrors(id string) ([]string, error) {
	rows, e := p.mirrorMap()
	if e != nil {
		return nil, e
	}
	return avquery.MirrorIDs(p.ctx, rows[id], func(_ context.Context, id string) (bool, error) { return p.exists(id) })
}

func (p *nativeEdit) upsertMirror(id, block string) (bool, error) {
	rows, e := p.mirrorMap()
	if e != nil {
		return false, e
	}
	old := len(rows[id])
	rows[id] = gulu.Str.RemoveDuplicatedElem(append(rows[id], block))
	after, e := msgpack.Marshal(rows)
	if e != nil {
		return false, e
	}
	return old != len(rows[id]) && old != 0, p.putFile("storage/av/blocks.msgpack", after)
}

func (p *nativeEdit) removeMirror(id, block string) (bool, error) {
	rows, e := p.mirrorMap()
	if e != nil {
		return false, e
	}
	old := rows[id]
	if old == nil {
		return false, nil
	}
	var next []string
	for _, member := range old {
		if member == block {
			continue
		}
		yes, e := p.exists(member)
		if e != nil {
			return false, e
		}
		if yes {
			next = append(next, member)
		}
	}
	rows[id] = next
	raw, e := msgpack.Marshal(rows)
	if e != nil {
		return false, e
	}
	return len(next) != len(old), p.putFile("storage/av/blocks.msgpack", raw)
}

func (p *nativeEdit) markRelated(id string) error {
	return editcore.CollectRelatedAttributeViews(p.ctx, id, p.sources, func(ids []string) error {
		for _, id := range ids {
			p.related[id] = true
		}
		return p.ctx.Err()
	})
}

func (p *nativeEdit) sources(id string) ([]string, error) {
	return av.ReadSourceAVIDs(p.ctx, id, func() ([]byte, error) { return p.readFile("storage/av/relations.msgpack") })
}

func (p *nativeEdit) relation(src, dest string, add bool) error {
	return av.MutateRelations(p.ctx, src, dest, add, func() ([]byte, error) { return p.readFile("storage/av/relations.msgpack") }, func(_, after []byte) error { return p.putFile("storage/av/relations.msgpack", after) })
}

func (p *nativeEdit) updateEnvironment() *editcore.AttributeViewUpdateEnvironment {
	return &editcore.AttributeViewUpdateEnvironment{Ctx: p.ctx, StrictDependencies: true, Read: p.readAV, Upsert: p.upsertMirror, Sources: p.sources, Groups: p.groups, Stage: p.saveAV, Reload: func(id string) error { return p.notice("attributeView", id, nil) }, Attrs: p.attrs, Tree: p.stage, AsyncRefresh: func(ids []string) error {
		for _, id := range ids {
			p.related[id] = true
		}
		return p.ctx.Err()
	}}
}

func (p *nativeEdit) flushTrees(trees []*parse.Tree) error {
	for _, t := range trees {
		if e := p.stage(t); e != nil {
			return e
		}
	}
	return p.ctx.Err()
}

func (p *nativeEdit) insertBindings(n *ast.Node, t *parse.Tree) error {
	return editcore.InsertAttributeViewBindings(n, t, p.bound)
}

func (p *nativeEdit) validateAssets(tree *parse.Tree) error {
	var result error
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if !enter || n.Type != ast.NodeLinkDest {
			return ast.WalkContinue
		}
		name := strings.TrimSpace(string(n.Tokens))
		if strings.HasPrefix(name, "assets/") && (strings.Contains(name, "\\") || strings.Contains(name, "../") || strings.ContainsRune(name, 0)) {
			result = ErrDocument
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return result
}

func (p *nativeEdit) refreshAVs(nodes map[string]*ast.Node) error {
	return editcore.UpdateAttributeViewBlockTextWithEnvironment(nodes, &editcore.AttributeViewTextEnvironment{Ctx: p.ctx, Read: p.readAV, Clone: func(a *av.AttributeView) (*av.AttributeView, error) {
		raw, e := json.Marshal(a)
		if e != nil {
			return nil, e
		}
		var v av.AttributeView
		if e = json.Unmarshal(raw, &v); e != nil {
			return nil, e
		}
		v.ReadAttributeView = p.readAV
		v.DurationFormatter = a.DurationFormatter
		return &v, nil
	}, Text: p.avText, Stage: p.saveAV, Reload: func(id string) error { return p.notice("attributeView", id, nil) }, Related: p.markRelated})
}

func (p *nativeEdit) finish() error {
	if e := p.refreshAVs(p.tx.Nodes); e != nil {
		return e
	}
	db, e := p.bundle.Database(indexdb.Main)
	if e != nil {
		return e
	}
	cache := anchor.Cache{Limit: 10240}
	defer cache.Close()
	e = anchor.Refresh(p.ctx, p.tx.Nodes, p.tx.Trees, anchor.Services{StrictLoad: true, Load: func(_ context.Context, id string) (*parse.Tree, error) {
		_, t, e := p.node(id)
		if t == nil && e == nil {
			e = editcore.ErrBlockNotFound
		}
		return t, e
	}, Embedded: indexdb.EmbeddedReference, References: func(ctx context.Context, n *ast.Node) ([]indexdb.RefRecord, []*ast.Node, error) {
		return anchor.References(ctx, n, func(c context.Context, id string) ([]indexdb.RefRecord, error) {
			return (indexdb.References{DB: db}).ByDefinition(c, id, false)
		})
	}, Text: func(c context.Context, n *ast.Node) (string, error) {
		return anchor.Text(c, n, p.r.configuration.Editor.BlockRefDynamicAnchorTextMaxLen, p.content)
	}, Set: func(n *ast.Node, text string) error { return anchor.SetText(n, text, cache.Put) }, Notify: func(_ context.Context, change anchor.Change) error {
		p.anchorText[[2]string{change.BlockID, change.DefinitionID}] = change.Text
		return p.notice("anchor", change.BlockID, change)
	}, IndexRefs: func(_ context.Context, t *parse.Tree) error { return p.stage(t) }, Stage: func(_ context.Context, t *parse.Tree) error { return p.stage(t) }, UpdateAVs: func(_ context.Context, nodes map[string]*ast.Node) error { return p.refreshAVs(nodes) }})
	if e != nil {
		return e
	}
	ids := []string{}
	for id := range p.related {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return editcore.FlushRelatedAttributeViews(p.ctx, ids, p.readAV, p.groups, p.saveAV, func(id string) error { return p.notice("attributeView", id, nil) }, false)
}
