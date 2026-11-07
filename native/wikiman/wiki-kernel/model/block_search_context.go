// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"maps"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type BlockSearchView struct {
	Blocks                                 []*Block
	MatchedBlocks, MatchedRoots, PageCount int
	DocumentMode                           bool
}

func (o *workspaceReadOwner) ignore(ctx context.Context, reference bool) ([]string, error) {
	name := ".siyuan/searchignore"
	if reference {
		name = ".siyuan/refsearchignore"
	}
	raw, e := o.raw(ctx, name)
	if indexrepair.IsSourceNotExist(e) {
		raw = nil
	} else if e != nil {
		return nil, e
	}
	return indexdb.SearchIgnoreLines(raw), ctx.Err()
}
func (o *workspaceReadOwner) searchView() blockview.Services {
	return blockview.Services{Strict: true, Blocks: o.r.main.Blocks, Tree: o.loadOptional, ReferenceCounts: (indexdb.References{DB: o.r.main.DB}).DefinitionCounts}
}

func FullTextSearchBlockContext(ctx context.Context, query string, boxes, paths []string, types map[string]bool, method, orderBy, groupBy, page, pageSize int) (blocks []*Block, matchedBlocks, matchedRoots, pageCount int, documentMode bool, err error) {
	request := indexdb.BlockSearchRequest{Query: query, Boxes: append([]string(nil), boxes...), Paths: append([]string(nil), paths...), Method: method, OrderBy: orderBy, Page: page, PageSize: pageSize, Types: maps.Clone(types)}
	out, e := runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (BlockSearchView, error) {
		out := BlockSearchView{Blocks: []*Block{}}
		if query == "" {
			return out, c.Err()
		}
		if o.settings.Search == nil {
			return out, errors.New("block_search_settings_missing")
		}
		ignore, e := o.ignore(c, false)
		if e != nil {
			return out, e
		}
		request.Ignore = ignore
		result, e := indexdb.SearchBlocks(c, o.r.main.DB, *o.settings.Search, request)
		if e != nil {
			return out, e
		}
		out.Blocks, e = blockview.FullText(c, result, groupBy, orderBy, o.settings.Search.CaseSensitive, o.searchView())
		if e != nil {
			return BlockSearchView{}, e
		}
		out.MatchedBlocks, out.MatchedRoots, out.PageCount, out.DocumentMode = result.MatchedBlocks, result.MatchedRoots, result.PageCount, result.DocumentMode
		return out, nil
	})
	if e != nil {
		return nil, 0, 0, 0, false, e
	}
	return out.Blocks, out.MatchedBlocks, out.MatchedRoots, out.PageCount, out.DocumentMode, nil
}
func RecentUpdatedBlocksContext(ctx context.Context) ([]*Block, error) {
	mobile := util.Container == util.ContainerIOS || util.Container == util.ContainerAndroid || util.Container == util.ContainerHarmony
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) ([]*Block, error) {
		if o.settings.Search == nil {
			return nil, errors.New("block_search_settings_missing")
		}
		ignore, e := o.ignore(c, false)
		if e != nil {
			return nil, e
		}
		return blockview.RecentUpdated(c, o.r.main.DB, *o.settings.Search, mobile, ignore)
	})
}
func GetBlockRefTextContext(ctx context.Context, id string) (string, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (string, error) {
		if e := FlushTxQueueContext(c); e != nil {
			return "", e
		}
		tree, e := o.load(c, id)
		if e != nil {
			return "", e
		}
		var node *ast.Node
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if c.Err() != nil {
				return ast.WalkStop
			}
			if enter && n.ID == id {
				node = n
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
		if node == nil {
			return "", ErrBlockNotFound
		}
		ast.Walk(node, func(n *ast.Node, enter bool) ast.WalkStatus {
			if c.Err() != nil {
				return ast.WalkStop
			}
			if enter && n.IsTextMarkType("inline-memo") {
				n.TextMarkInlineMemoContent = ""
			}
			return ast.WalkContinue
		})
		if o.settings.Editor == nil {
			return "", errors.New("anchor_editor_settings_missing")
		}
		return anchor.Text(c, node, o.settings.Editor.BlockRefDynamicAnchorTextMaxLen, o.content())
	})
}
