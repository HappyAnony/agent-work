// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeBacklinks(ctx context.Context, q blockview.BacklinkRequest) (out blockview.BacklinkResult, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if q.DefaultChildren {
			if r.configuration.Editor == nil {
				return ErrDocument
			}
			q.Children = r.configuration.Editor.BacklinkContainChildren
		}
		services, e := r.blockReadServices(ctx, w, b)
		if e != nil {
			return e
		}
		out, e = blockview.ReadBacklinks(ctx, q, services)
		return e
	})
	if err != nil {
		return blockview.BacklinkResult{}, err
	}
	return
}

func (r *resources) blockReadServices(ctx context.Context, w *WorkspaceContext, b *indexdb.Bundle) (blockview.ReadServices, error) {
	if !r.blockTreeReady {
		if e := r.rebuildBlockTree(ctx, w); e != nil {
			return blockview.ReadServices{}, e
		}
	}
	db, e := b.Database(indexdb.Main)
	if e != nil {
		return blockview.ReadServices{}, e
	}
	registryDB, e := b.Database(indexdb.BlockTree)
	if e != nil {
		return blockview.ReadServices{}, e
	}
	parser, e := r.documentInfoParser(ctx, w)
	if e != nil {
		return blockview.ReadServices{}, e
	}
	position := r.positionServices(w, b)

	return blockview.BacklinkServices{DB: db, Search: r.configuration.Search, Parser: parser, Content: r.readBlockContent(ctx, w), Notebooks: r.configurationBusiness.Notebooks,
		Load: func(c context.Context, id string) (*parse.Tree, error) {
			trees, e := r.documentInfoTrees(c, w, []string{id})
			return trees[id], e
		}, Trees: func(c context.Context, ids []string) (map[string]*parse.Tree, error) {
			return r.documentInfoTrees(c, w, ids)
		},
		Location: func(c context.Context, id string) (*indexdb.BlockTreeRecord, error) {
			rows, e := (indexdb.BlockTrees{DB: registryDB}).Query(c, indexdb.BlockTreeFilter{Kind: "id", Value: id})
			if e != nil || len(rows) == 0 {
				return nil, e
			}
			return &rows[0], nil
		},
		Breadcrumb: func(c context.Context, n *ast.Node, embed bool, headingMode int) ([]*blockview.BlockPath, error) {
			return docquery.BreadcrumbFromNode(c, n, n.Box, n.Path, docquery.PositionRequest{Embed: embed, HeadingMode: headingMode}, position)
		},
		Render: func(c context.Context, nodes []*ast.Node, p *lute.Lute) (string, error) {
			return docview.RenderNodes(c, nodes, p, func(c context.Context, ids string) (string, error) {
				return docview.AVNames(c, ids, position.AVName, func(c context.Context) (string, error) {
					return r.configurationBusiness.Language(r.configuration.Lang, 105, false, r.configuration.CloudRegion), c.Err()
				})
			})
		},
	}, nil
}
