// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/conf"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeReadBlocks(ctx context.Context, q docquery.BlocksRequest) (out docquery.BlocksResult, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		content := r.readBlockContent(ctx, w)
		out, e = docquery.Blocks(ctx, q, docquery.BlocksServices{Render: func(c context.Context, nodes []*ast.Node, p *lute.Lute) (string, error) {
			return docview.RenderNodes(c, nodes, p, func(c context.Context, ids string) (string, error) {
				return docview.AVNames(c, ids, func(c context.Context, id string) (string, error) {
					name, _, e := r.readNativeAV(c, w, id)
					return name, e
				}, func(c context.Context) (string, error) {
					return r.configurationBusiness.Language(r.configuration.Lang, 105, false, r.configuration.CloudRegion), c.Err()
				})
			})
		}, Parser: parser, Content: content, Missing: ErrDocument, Load: func(c context.Context, id string) (*parse.Tree, error) {
			trees, e := r.documentInfoTrees(c, w, []string{id})
			return trees[id], e
		}})
		return e
	})
	if err != nil {
		return docquery.BlocksResult{}, err
	}
	return
}

// NativeGetBlock retains the original API's Block presentation rather than
// exposing the intermediate SQL projection as an API block.
func (l *Lease) NativeGetBlock(ctx context.Context, id string) (*blockview.Block, error) {
	value, e := l.NativeReadBlocks(ctx, docquery.BlocksRequest{Mode: "block", ID: id})
	if e != nil {
		return nil, e
	}
	settings, e := l.Configuration(ctx)
	if e != nil {
		return nil, e
	}
	if settings.Search == nil {
		return nil, conf.ErrBusinessUnbound
	}
	return blockview.FromRow(value.Block, "", 0, settings.Search.CaseSensitive), nil
}
