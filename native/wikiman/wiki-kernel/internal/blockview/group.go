// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"path"
	"sort"
	"strings"
	"wiki-kernel/internal/indexdb"
)

type Services struct {
	Strict          bool
	Blocks          func(context.Context, []string) ([]*indexdb.BlockRecord, error)
	Tree            func(context.Context, string) (*parse.Tree, error)
	ReferenceCounts func(context.Context, []string) (map[string]int, error)
}

func FullText(ctx context.Context, result indexdb.BlockSearchResult, groupBy, orderBy int, caseSensitive bool, services Services) (ret []*Block, err error) {
	if services.Blocks == nil || services.Tree == nil || services.ReferenceCounts == nil {
		return nil, errors.New("search_view_dependency_unbound")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	beforeLen := 36
	var raw []*indexdb.BlockRecord
	for _, r := range result.Blocks {
		b := r
		raw = append(raw, &b)
	}
	blocks := FromRows(&raw, result.Terms, beforeLen, caseSensitive)
	switch groupBy {
	case 0: // 不分组
		ret = blocks
	case 1: // 按文档分组
		rootMap := map[string]bool{}
		var rootIDs []string
		contentSorts := map[string]int{}
		for _, b := range blocks {
			if _, ok := rootMap[b.RootID]; !ok {
				rootMap[b.RootID] = true
				rootIDs = append(rootIDs, b.RootID)
				tree, e := services.Tree(ctx, b.RootID)
				if e != nil && services.Strict {
					return nil, e
				}
				if nil == tree {
					continue
				}

				if 5 == orderBy { // 按内容顺序（仅在按文档分组时）
					sortVal := 0
					ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
						if !entering || !n.IsBlock() {
							return ast.WalkContinue
						}

						contentSorts[n.ID] = sortVal
						sortVal++
						return ast.WalkContinue
					})
				}
			}
		}

		sqlRoots, e := services.Blocks(ctx, rootIDs)
		if e != nil {
			return nil, e
		}
		for _, root := range sqlRoots {
			if root == nil {
				return nil, errors.New("search_root_missing")
			}
		}
		roots := FromRows(&sqlRoots, "", beforeLen, caseSensitive)
		for _, root := range roots {
			for _, b := range blocks {
				if 5 == orderBy { // 按内容顺序（仅在按文档分组时）
					b.Sort = contentSorts[b.ID]
				}
				if b.RootID == root.ID {
					root.Children = append(root.Children, b)
				}
			}

			switch orderBy {
			case 1: //按创建时间升序
				sort.Slice(root.Children, func(i, j int) bool { return root.Children[i].Created < root.Children[j].Created })
			case 2: // 按创建时间降序
				sort.Slice(root.Children, func(i, j int) bool { return root.Children[i].Created > root.Children[j].Created })
			case 3: // 按更新时间升序
				sort.Slice(root.Children, func(i, j int) bool { return root.Children[i].Updated < root.Children[j].Updated })
			case 4: // 按更新时间降序
				sort.Slice(root.Children, func(i, j int) bool { return root.Children[i].Updated > root.Children[j].Updated })
			case 5: // 按内容顺序（仅在按文档分组时）
				sort.Slice(root.Children, func(i, j int) bool { return root.Children[i].Sort < root.Children[j].Sort })
			default: // 按块类型（默认）
				sort.Slice(root.Children, func(i, j int) bool { return root.Children[i].Sort < root.Children[j].Sort })
			}
		}

		switch orderBy {
		case 1: //按创建时间升序
			sort.Slice(roots, func(i, j int) bool { return roots[i].Created < roots[j].Created })
		case 2: // 按创建时间降序
			sort.Slice(roots, func(i, j int) bool { return roots[i].Created > roots[j].Created })
		case 3: // 按更新时间升序
			sort.Slice(roots, func(i, j int) bool { return roots[i].Updated < roots[j].Updated })
		case 4: // 按更新时间降序
			sort.Slice(roots, func(i, j int) bool { return roots[i].Updated > roots[j].Updated })
		case 5: // 按内容顺序（仅在按文档分组时）
			// 都是文档，按更新时间降序
			sort.Slice(roots, func(i, j int) bool { return roots[i].IAL["updated"] > roots[j].IAL["updated"] })
		case 6, 7: // 按相关度
		// 已在 ORDER BY 中处理
		default: // 按块类型（默认）
			// 都是文档，不需要再次排序
		}
		ret = roots
	default:
		ret = blocks
	}
	if 1 > len(ret) {
		ret = []*Block{}
	}

	if 0 == groupBy {
		FilterSelfHPath(ret)
	}

	var nodeIDs []string
	for _, b := range ret {
		if 0 == groupBy {
			nodeIDs = append(nodeIDs, b.ID)
		} else {
			for _, c := range b.Children {
				nodeIDs = append(nodeIDs, c.ID)
			}
		}
	}

	refCount, e := services.ReferenceCounts(ctx, nodeIDs)
	if e != nil {
		return nil, e
	}
	for _, b := range ret {
		if 0 == groupBy {
			b.RefCount = refCount[b.ID]
		} else {
			for _, c := range b.Children {
				c.RefCount = refCount[c.ID]
			}
		}
	}
	return ret, ctx.Err()
}

func FilterSelfHPath(blocks []*Block) {
	// 简化搜索结果列表中的文档块路径 Simplify document block paths in search results https://github.com/siyuan-note/siyuan/issues/13364
	// 文档块不显示自己的路径（最后一层）

	for _, b := range blocks {
		if b.IsDoc() {
			b.HPath = strings.TrimSuffix(b.HPath, path.Base(b.HPath))
		}
	}
}
