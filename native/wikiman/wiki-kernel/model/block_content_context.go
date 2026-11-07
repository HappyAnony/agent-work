// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docquery"
)

func ReadBlocksContext(ctx context.Context, q docquery.BlocksRequest) (docquery.BlocksResult, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (docquery.BlocksResult, error) {
		return docquery.Blocks(c, q, docquery.BlocksServices{Render: o.renderNodes, Load: o.loadOptional, Parser: o.parser, Content: o.content(), Missing: ErrBlockNotFound})
	})
}
func GetBlockContext(ctx context.Context, id string, supplied ...*parse.Tree) (*Block, error) {
	out, e := runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (*Block, error) {
		load := o.loadOptional
		if len(supplied) > 0 && supplied[0] != nil {
			tree := supplied[0]
			load = func(c context.Context, _ string) (*parse.Tree, error) { return tree, c.Err() }
		}
		v, e := docquery.Blocks(c, docquery.BlocksRequest{Mode: "block", ID: id}, docquery.BlocksServices{Render: o.renderNodes, Load: load, Parser: o.parser, Content: o.content(), Missing: ErrBlockNotFound})
		if e != nil || v.Block == nil {
			return nil, e
		}
		if o.settings.Search == nil {
			return nil, errors.New("block_search_settings_missing")
		}
		return blockview.FromRow(v.Block, "", 0, o.settings.Search.CaseSensitive), nil
	})
	return out, e
}
func GetChildBlocksContext(ctx context.Context, id string) ([]*ChildBlock, error) {
	v, e := ReadBlocksContext(ctx, docquery.BlocksRequest{Mode: "children", ID: id})
	return v.Children, e
}
func GetTailChildBlocksContext(ctx context.Context, id string, n int) ([]*ChildBlock, error) {
	v, e := ReadBlocksContext(ctx, docquery.BlocksRequest{Mode: "children", ID: id, Tail: true, Limit: n})
	return v.Children, e
}
func GetBlockKramdownContext(ctx context.Context, id, mode string) (string, error) {
	if mode != "md" {
		mode = "kramdown"
	}
	v, e := ReadBlocksContext(ctx, docquery.BlocksRequest{Mode: mode, ID: id})
	return v.Text, e
}
func GetHeadingChildrenIDsContext(ctx context.Context, id string) ([]string, error) {
	v, e := ReadBlocksContext(ctx, docquery.BlocksRequest{Mode: "heading-ids", ID: id})
	return v.IDs, e
}
func GetHeadingChildrenDOMContext(ctx context.Context, id string, removeFold bool) (string, error) {
	v, e := ReadBlocksContext(ctx, docquery.BlocksRequest{Mode: "heading-dom", ID: id, RemoveFold: removeFold})
	return v.Text, e
}
