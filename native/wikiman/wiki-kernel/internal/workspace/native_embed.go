// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"slices"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/conf"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/wikistore"
)

// NativeEmbeds returns detached rendered values. The original plain text stays
// in Block.Markdown for an explicit owner-derived update; source is never saved.
func (l *Lease) NativeEmbeds(ctx context.Context, q blockview.EmbedRequest) (out []*blockview.EmbedBlock, err error) {
	q.IncludeIDs = slices.Clone(q.IncludeIDs)
	q.ExcludeIDs = slices.Clone(q.ExcludeIDs)
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		s, e := l.workspace.resources.blockReadServices(ctx, l.workspace, b)
		if e != nil {
			return e
		}
		out, e = blockview.ReadEmbeds(ctx, q, s)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

func (l *Lease) NativeEntryEmbeds(ctx context.Context, entry wikistore.EntryDocument, q blockview.EmbedRequest) (out []*blockview.EmbedBlock, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.Search == nil {
			return indexdb.ErrProjection
		}
		tree, e := r.locatedTree(ctx, w, entry.BoxID, entry.DocID)
		if e != nil {
			return e
		}
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		projection, e := entryReadProjection(ctx, tree, parser, r.configuration.Search.CaseSensitive)
		if e != nil {
			return e
		}
		defer projection.Close()
		db, e := projection.Database(indexdb.Main)
		if e != nil {
			return e
		}
		content := entryProjectionContent()
		noName := func(c context.Context, _ string) (string, error) { return "", c.Err() }
		position := docquery.PositionServices{
			Notebook: func(c context.Context, box string) (string, bool, error) {
				return "", box == entry.BoxID, c.Err()
			},
			RootPath: func(c context.Context, _, _ string) (string, error) { return tree.HPath, c.Err() },
			AVName:   noName,
			Text: func(c context.Context, n *ast.Node, exclude []string, lineBreak bool) (string, error) {
				return docquery.BlockText(c, n, exclude, lineBreak, func(c context.Context, n *ast.Node, exclude []string) (string, error) {
					return indexdb.StaticContent(c, n, exclude, false, false, false, content)
				})
			},
		}
		out, e = blockview.ReadEmbeds(ctx, q, blockview.ReadServices{
			DB: db, Search: r.configuration.Search, Parser: parser, Content: content,
			Load: func(c context.Context, id string) (*parse.Tree, error) {
				if id != entry.DocID {
					return nil, ErrIdentity
				}
				return tree, c.Err()
			},
			Location: func(c context.Context, id string) (*indexdb.BlockTreeRecord, error) {
				n := docview.GetNodeInTree(tree, id)
				if n == nil {
					return nil, c.Err()
				}
				row := &indexdb.BlockTreeRecord{ID: id, RootID: entry.DocID, BoxID: entry.BoxID, Path: tree.Path, HPath: tree.HPath}
				if n.Parent != nil {
					row.ParentID = n.Parent.ID
				}
				return row, c.Err()
			},
			Notebooks: func(c context.Context) ([]conf.Notebook, error) { return nil, c.Err() },
			Breadcrumb: func(c context.Context, n *ast.Node, embed bool, mode int) ([]*blockview.BlockPath, error) {
				return docquery.BreadcrumbFromNode(c, n, tree.Box, tree.Path, docquery.PositionRequest{Embed: embed, HeadingMode: mode}, position)
			},
			Render: func(c context.Context, nodes []*ast.Node, parser *lute.Lute) (string, error) {
				return docview.RenderNodes(c, nodes, parser, noName)
			},
		})
		return e
	})
	return
}
