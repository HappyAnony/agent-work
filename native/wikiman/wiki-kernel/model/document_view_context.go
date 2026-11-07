// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"maps"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/pageprefs"
)

func DocumentViewContext(ctx context.Context, q docview.Request) (docview.Result, error) {
	q.QueryTypes = maps.Clone(q.QueryTypes)
	q.OriginalRefs = maps.Clone(q.OriginalRefs)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (docview.Result, error) {
		if o.settings.Editor == nil || o.settings.Search == nil {
			return docview.Result{}, errors.New("document_view_settings_missing")
		}
		editorConf, searchConf := *o.settings.Editor, *o.settings.Search
		content := o.content()
		options := docview.VirtualOptions{Enabled: editorConf.VirtualBlockRef, CaseSensitive: searchConf.CaseSensitive, Include: editorConf.VirtualBlockRefInclude, Exclude: editorConf.VirtualBlockRefExclude}
		dictionary := &docview.VirtualDictionary{Options: options, Content: content, Read: func(c context.Context) ([]string, error) {
			ignore, e := o.ignore(c, false)
			if e != nil {
				return nil, e
			}
			refs, e := o.ignore(c, true)
			if e != nil {
				return nil, e
			}
			return (indexdb.References{DB: o.r.main.DB}).VirtualKeywords(c, searchConf.VirtualRefName, searchConf.VirtualRefAlias, searchConf.VirtualRefAnchor, searchConf.VirtualRefDoc, ignore, refs)
		}}
		s := &docview.Services{Ctx: c, CaseSensitive: searchConf.CaseSensitive, DynamicLoadBlocks: editorConf.DynamicLoadBlocks, BlockMissing: ErrBlockNotFound, TreeMissing: ErrTreeNotFound, Wait: FlushTxQueueContext, Load: o.load,
			Parser: func(c context.Context) (*lute.Lute, error) { return o.parser, o.r.guard(c) }, References: (indexdb.References{DB: o.r.main.DB}).RootChildrenCounts, VirtualKeywords: dictionary.Keys, AvNames: o.avNames,
			Backlink: func(c context.Context, n *ast.Node, refs map[string]string) ([]*ast.Node, bool, error) {
				nodes, expand := docview.BacklinkNodes(n, refs)
				return nodes, expand, c.Err()
			},
			Keywords: func(c context.Context, query string, types map[string]bool, method int, root string) ([]string, error) {
				return indexdb.DocumentKeywords(c, o.r.main.DB, searchConf, query, types, method, root)
			},
			Virtual: func(c context.Context, n *ast.Node, unlinks *[]*ast.Node, keys []string, counts map[string]int, p *lute.Lute) (bool, error) {
				return docview.ProcessVirtual(c, n, unlinks, keys, counts, p, options, func(c context.Context, n *ast.Node) (string, error) {
					return anchor.Text(c, n, editorConf.BlockRefDynamicAnchorTextMaxLen, content)
				})
			},
			Render: func(c context.Context, t *parse.Tree, p *lute.Lute) (string, error) {
				if e := o.r.guard(c); e != nil {
					return "", e
				}
				return p.Tree2BlockDOM(t, p.RenderOptions), c.Err()
			},
			Recent: func(c context.Context, v docview.Recent) (err error) {
				o.base.owner.m.Lock()
				readonly := o.base.owner.persistenceReadOnly
				o.base.owner.m.Unlock()
				if readonly {
					return o.r.guard(c)
				}
				b, closeBackend, e := o.base.preferenceBackend(c, false, o.r.registry)
				if e != nil {
					return e
				}
				defer func() { err = errors.Join(err, closeBackend()) }()
				value, e := json.Marshal(pageprefs.RecentDoc{RootID: v.ID, Icon: v.Icon, Title: v.Title})
				if e != nil {
					return e
				}
				_, e = o.base.owner.preferencesService().Do(c, pageprefs.Request{Action: "recent.record", Value: value}, b)
				return e
			},
		}
		return docview.Read(q, s)
	})
}
func GetDocContext(ctx context.Context, startID, endID, id string, index int, query string, types map[string]bool, method, mode, size int, backlink bool, refs map[string]string, highlight bool, dynamicSize ...bool) (int, string, string, string, string, string, bool, bool, string, string, bool, []string, error) {
	useDynamic := len(dynamicSize) > 0 && dynamicSize[0]
	v, e := DocumentViewContext(ctx, docview.Request{UseDynamicSize: useDynamic, StartID: startID, EndID: endID, ID: id, Index: index, Query: query, QueryTypes: types, QueryMethod: method, Mode: mode, Size: size, Backlink: backlink, OriginalRefs: refs, Highlight: highlight})
	return v.BlockCount, v.DOM, v.ParentID, v.Parent2ID, v.RootID, v.Type, v.EOF, v.Scroll, v.Box, v.Path, v.BacklinkExpand, v.Keywords, e
}
