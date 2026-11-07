// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
	"sort"
	"strings"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
)

type EmbedBlock struct {
	Block      *Block       `json:"block"`
	BlockPaths []*BlockPath `json:"blockPaths"`
}
type EmbedRequest struct {
	SQL                    string
	IncludeIDs, ExcludeIDs []string
	Query                  bool
	HeadingMode            int
	Breadcrumb             bool
}

func ReadEmbeds(ctx context.Context, q EmbedRequest, s ReadServices) (out []*EmbedBlock, err error) {
	scope := &blockReadScope{ctx: ctx, s: s, trees: map[string]*parse.Tree{}}
	defer func() {
		if v := recover(); v != nil {
			if scope.err == nil {
				scope.err = fmt.Errorf("%w: embed projection (%T)", indexdb.ErrProjection, v)
			}
		}
		if scope.err != nil {
			out = nil
			err = scope.err
		}
	}()
	if ctx == nil || s.DB == nil || s.Search == nil || s.Parser == nil || s.Load == nil || s.Render == nil || s.Location == nil || s.Notebooks == nil || s.Breadcrumb == nil {
		return nil, errors.New("embed_dependencies_unbound")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	out = []*EmbedBlock{}
	q.IncludeIDs = slices.Clone(q.IncludeIDs)
	q.ExcludeIDs = slices.Clone(q.ExcludeIDs)
	var rows []*indexdb.BlockRecord
	if q.Query {
		if s.Search.Limit <= 0 {
			return nil, errors.New("embed_search_limit_invalid")
		}
		rows = scope.selectRows(q.SQL, 1, s.Search.Limit)
	} else {
		if len(q.IncludeIDs) == 0 {
			return out, nil
		}
		quoted := []string{}
		order := map[string]int{}
		for i, id := range q.IncludeIDs {
			quoted = append(quoted, "'"+strings.ReplaceAll(id, "'", "''")+"'")
			order[id] = i
		}
		rows = scope.selectRows("SELECT * FROM blocks WHERE id IN ("+strings.Join(quoted, ",")+")", 1, 1024)
		sort.SliceStable(rows, func(i, j int) bool { return order[rows[i].ID] < order[rows[j].ID] })
	}
	if scope.err != nil {
		return nil, scope.err
	}
	filtered := []*indexdb.BlockRecord{}
	for _, b := range rows {
		if b.Type != "query_embed" && !slices.Contains(q.ExcludeIDs, b.ID) {
			filtered = append(filtered, b)
		}
	}
	trees := map[string]*parse.Tree{}
	for _, b := range filtered {
		if !scope.guard() {
			return nil, scope.err
		}
		if trees[b.RootID] == nil {
			tree, e := s.Load(ctx, b.RootID)
			if e != nil {
				return nil, e
			}
			if tree == nil {
				return nil, errors.New("embed_source_missing")
			}
			trees[b.RootID] = tree
		}
		if len(trees) >= 128 {
			break
		}
	}
	boxes := []string{}
	for _, b := range filtered {
		if !scope.guard() {
			return nil, scope.err
		}
		block, paths := scope.getEmbeddedBlock(trees, b, q.HeadingMode, q.Breadcrumb)
		if scope.err != nil {
			return nil, scope.err
		}
		if block != nil {
			out = append(out, &EmbedBlock{Block: block, BlockPaths: paths})
			boxes = append(boxes, block.Box)
		}
	}
	names := scope.boxNames(boxes)
	if scope.err != nil {
		return nil, scope.err
	}
	for _, v := range out {
		v.Block.HPath = names[v.Block.Box] + v.Block.HPath
	}
	scope.fail(ctx.Err())
	return
}
func (scope *blockReadScope) content(nodes []*ast.Node) string {
	var out strings.Builder
	for _, n := range nodes {
		children := []*ast.Node{n}
		if n.Type == ast.NodeDocument {
			children = nil
			for c := n.FirstChild; c != nil; c = c.Next {
				children = append(children, c)
			}
		}
		for _, c := range children {
			if !scope.guard() {
				return ""
			}
			text, e := indexdb.StaticContent(scope.ctx, c, nil, false, false, false, scope.s.Content)
			scope.fail(e)
			if e != nil {
				return ""
			}
			out.WriteString(text)
		}
	}
	return out.String()
}
func (scope *blockReadScope) getEmbeddedBlock(trees map[string]*parse.Tree, sqlBlock *indexdb.BlockRecord, headingMode int, breadcrumb bool) (block *Block, blockPaths []*BlockPath) {
	tree, _ := trees[sqlBlock.RootID]
	if nil == tree {
		var e error
		tree, e = scope.s.Load(scope.ctx, sqlBlock.RootID)
		scope.fail(e)
	}
	if nil == tree {
		return
	}
	def := docview.GetNodeInTree(tree, sqlBlock.ID)
	if nil == def {
		return
	}

	var unlinks, nodes []*ast.Node
	ast.Walk(def, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !scope.guard() {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if ast.NodeHeading == n.Type {
			if "1" == n.IALAttr("fold") {
				children := docview.HeadingChildren(n)
				for _, c := range children {
					unlinks = append(unlinks, c)
				}
			}
		}
		return ast.WalkContinue
	})
	for _, n := range unlinks {
		n.Unlink()
	}
	// headingMode: 0=显示标题与下方的块，1=仅显示标题，2=仅显示标题下方的块
	if ast.NodeHeading == def.Type {
		if 1 == headingMode {
			// 仅显示标题
			nodes = append(nodes, def)
		} else if 2 == headingMode {
			// 仅显示标题下方的块（去除标题）
			if "1" != def.IALAttr("fold") {
				children := docview.HeadingChildren(def)
				for _, c := range children {
					if "1" == c.IALAttr("heading-fold") {
						// 嵌入块包含折叠标题时不应该显示其下方块 https://github.com/siyuan-note/siyuan/issues/4765
						continue
					}
					nodes = append(nodes, c)
				}
			}
		} else {
			// 0: 显示标题与下方的块
			nodes = append(nodes, def)
			if "1" != def.IALAttr("fold") {
				children := docview.HeadingChildren(def)
				for _, c := range children {
					if "1" == c.IALAttr("heading-fold") {
						// 嵌入块包含折叠标题时不应该显示其下方块 https://github.com/siyuan-note/siyuan/issues/4765
						continue
					}
					nodes = append(nodes, c)
				}
			}
		}
	} else {
		// 非标题块，直接添加
		nodes = append(nodes, def)
	}

	b := scope.location(def.ID)
	if nil == b {
		return
	}

	// 嵌入块查询结果中显示块引用计数 https://github.com/siyuan-note/siyuan/issues/7191
	scope.fillCounts(nodes)

	luteEngine := scope.newParser()
	luteEngine.RenderOptions.ProtyleContenteditable = false // 不可编辑
	dom := scope.render(nodes, luteEngine)
	content := scope.content(nodes)
	block = &Block{Box: def.Box, Path: def.Path, HPath: b.HPath, ID: def.ID, RootID: tree.ID, ParentID: b.ParentID, Type: def.Type.String(), Content: dom, Markdown: content /* 这里使用 Markdown 字段来临时存储 content */}

	if "" != sqlBlock.IAL {
		block.IAL = map[string]string{}
		ialStr := strings.TrimPrefix(sqlBlock.IAL, "{:")
		ialStr = strings.TrimSuffix(ialStr, "}")
		ial := parse.Tokens2IAL([]byte(ialStr))
		for _, kv := range ial {
			block.IAL[kv[0]] = kv[1]
		}
	}

	if breadcrumb {
		blockPaths = scope.breadcrumb(def, nil, true, headingMode)
	}
	if 1 > len(blockPaths) {
		blockPaths = []*BlockPath{}
	}
	return
}
