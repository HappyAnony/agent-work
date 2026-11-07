// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	util2 "github.com/88250/lute/util"

	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/search"
)

func getDocCore(startID, endID, id string, index int, query string, queryTypes map[string]bool, queryMethod, mode int, size int, isBacklink bool, originalRefBlockIDs map[string]string, highlight bool, e *Services) (
	blockCount int, dom, parentID, parent2ID, rootID, typ string, eof, scroll bool, boxID, docPath string, isBacklinkExpand bool, keywords []string, err error) {
	if err = e.guard(func() error { return e.Wait(e.Ctx) }); err != nil {
		return
	}

	inputIndex := index
	var tree *parse.Tree
	err = e.guard(func() (loadErr error) { tree, loadErr = e.Load(e.Ctx, id); return })
	if err != nil {
		if e.BlockMissing == err {
			if 0 == mode {
				err = e.TreeMissing // 初始化打开文档时如果找不到则关闭编辑器
			}
		}
		return
	}
	if nil == tree {
		err = e.BlockMissing
		return
	}

	if tree.Root == nil {
		err = errors.New("get_doc_invalid_tree")
		return
	}
	if !e.Legacy {
		restore := snapshotGetDocTree(tree)
		defer restore()
	}
	var luteEngine *lute.Lute
	if err = e.guard(func() (parserErr error) { luteEngine, parserErr = e.Parser(e.Ctx); return }); err != nil {
		return
	}
	if luteEngine == nil || luteEngine.ParseOptions == nil || luteEngine.RenderOptions == nil {
		err = errors.New("get_doc_parser_unbound")
		return
	}
	engineCopy := *luteEngine
	renderCopy := *luteEngine.RenderOptions
	engineCopy.RenderOptions = &renderCopy
	luteEngine = &engineCopy
	node := GetNodeInTree(tree, id)
	if nil == node {
		// Unable to open the doc when the block pointed by the scroll position does not exist https://github.com/siyuan-note/siyuan/issues/9030
		node = GetNodeInTree(tree, tree.Root.ID)
		if nil == node {
			err = e.BlockMissing
			return
		}
	}

	located := false
	isDoc := ast.NodeDocument == node.Type
	isHeading := ast.NodeHeading == node.Type

	boxID = node.Box
	docPath = node.Path
	if isDoc {
		if 4 == mode { // 加载文档末尾
			node = node.LastChild
			located = true
			// 重新计算 index
			ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
				if e.Ctx.Err() != nil {
					return ast.WalkStop
				}
				if !entering {
					return ast.WalkContinue
				}

				index++
				return ast.WalkContinue
			})
		} else {
			node = node.FirstChild
		}
		typ = ast.NodeDocument.String()
		idx := 0
		if 0 < index {
			ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
				if !entering || !n.IsChildBlockOf(tree.Root, 1) {
					return ast.WalkContinue
				}

				idx++
				if index == idx {
					node = n.DocChild()
					if "1" == node.IALAttr("heading-fold") {
						// 加载到折叠标题下方块的话需要回溯到上方标题块
						for h := node.Previous; nil != h; h = h.Previous {
							if "1" == h.IALAttr("fold") {
								node = h
								break
							}
						}
					}
					located = true
					return ast.WalkStop
				}
				return ast.WalkContinue
			})
		}
	} else {
		if 0 == index && 0 != mode {
			// 非文档且没有指定 index 时需要计算 index
			ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
				if e.Ctx.Err() != nil {
					return ast.WalkStop
				}
				if !entering {
					return ast.WalkContinue
				}

				index++
				if id == n.ID {
					node = n.DocChild()
					located = true
					return ast.WalkStop
				}
				return ast.WalkContinue
			})
		}
	}

	if 1 < index && !located {
		count := 0
		ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if e.Ctx.Err() != nil {
				return ast.WalkStop
			}
			if !entering {
				return ast.WalkContinue
			}

			count++
			if index == count {
				node = n.DocChild()
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
	}

	if node == nil || (node.Type != ast.NodeDocument && node.Parent == nil) {
		err = e.BlockMissing
		return
	}
	blockCount = tree.DocBlockCount()
	if ast.NodeDocument == node.Type {
		parentID = node.ID
		parent2ID = parentID
	} else {
		parentID = node.Parent.ID
		parent2ID = parentID
		tmp := node
		if ast.NodeListItem == node.Type {
			// 列表项聚焦返回和面包屑保持一致 https://github.com/siyuan-note/siyuan/issues/4914
			tmp = node.Parent
		}
		if headingParent := HeadingParent(tmp); nil != headingParent {
			parent2ID = headingParent.ID
		}
	}
	rootID = tree.Root.ID
	if !isDoc {
		typ = node.Type.String()
	}

	// 判断是否需要显示动态加载滚动条 https://github.com/siyuan-note/siyuan/issues/7693
	childCount := 0
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if e.Ctx.Err() != nil {
			return ast.WalkStop
		}
		if !entering {
			return ast.WalkContinue
		}

		if 1 > childCount {
			childCount = 1
		} else {
			childCount += CountBlockNodes(n)
		}

		if childCount > e.DynamicLoadBlocks {
			scroll = true
			return ast.WalkStop
		}
		return ast.WalkContinue
	})

	var nodes []*ast.Node
	if isBacklink {
		// 引用计数浮窗请求，需要按照反链逻辑组装 https://github.com/siyuan-note/siyuan/issues/6853
		if err = e.guard(func() (backErr error) {
			nodes, isBacklinkExpand, backErr = e.Backlink(e.Ctx, node, originalRefBlockIDs)
			return
		}); err != nil {
			return
		}
	} else {
		// 如果同时存在 startID 和 endID，并且是动态加载的情况，则只加载 startID 和 endID 之间的块 [startID, endID]
		if "" != startID && "" != endID && scroll {
			nodes, eof, err = LoadNodesByStartEnd(e.Ctx, tree, startID, endID, e.DynamicLoadBlocks)
			if err != nil {
				return
			}
			if 1 > len(nodes) {
				// 按 mode 加载兜底
				nodes, eof, err = LoadNodesByMode(e.Ctx, node, inputIndex, mode, size, isDoc, isHeading)
				if err != nil {
					return
				}
			} else {
				// 文档块没有指定 index 时需要计算 index，否则初次打开文档时 node-index 会为 0，导致首次 Ctrl+Home 无法回到顶部
				ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
					if e.Ctx.Err() != nil {
						return ast.WalkStop
					}
					if !entering {
						return ast.WalkContinue
					}

					index++
					if nodes[0].ID == n.ID {
						return ast.WalkStop
					}
					return ast.WalkContinue
				})
			}
		} else {
			nodes, eof, err = LoadNodesByMode(e.Ctx, node, inputIndex, mode, size, isDoc, isHeading)
			if err != nil {
				return
			}
		}
	}

	var refCount map[string]int
	if err = e.guard(func() (refErr error) { refCount, refErr = e.References(e.Ctx, rootID); return }); err != nil {
		return
	}
	refCount = maps.Clone(refCount)
	var virtualBlockRefKeywords []string
	if err = e.guard(func() (virtualErr error) {
		virtualBlockRefKeywords, virtualErr = e.VirtualKeywords(e.Ctx, tree.Root)
		return
	}); err != nil {
		return
	}
	virtualBlockRefKeywords = slices.Clone(virtualBlockRefKeywords)

	subTree := &parse.Tree{ID: rootID, Root: &ast.Node{Type: ast.NodeDocument}, Marks: tree.Marks}

	query = indexdb.FilterQueryInvisible(query)
	if "" != query && (0 == queryMethod || 1 == queryMethod || 3 == queryMethod) { // 只有关键字、查询语法和正则表达式搜索支持高亮
		if err = e.guard(func() (keyErr error) {
			keywords, keyErr = e.Keywords(e.Ctx, query, maps.Clone(queryTypes), queryMethod, rootID)
			return
		}); err != nil {
			return
		}
		keywords = slices.Clone(keywords)
	}

	existKeywords := 0 < len(keywords)
	for _, n := range nodes {
		var unlinks []*ast.Node
		ast.Walk(n, func(n *ast.Node, entering bool) ast.WalkStatus {
			if e.Ctx.Err() != nil {
				return ast.WalkStop
			}
			if !entering {
				return ast.WalkContinue
			}

			if "1" == n.IALAttr("heading-fold") {
				// 折叠标题下被引用的块无法悬浮查看
				// The referenced block under the folded heading cannot be hovered to view https://github.com/siyuan-note/siyuan/issues/9582
				if (0 != mode && id != n.ID) || isDoc {
					unlinks = append(unlinks, n)
					return ast.WalkContinue
				}
			}

			if avs := n.IALAttr(avmodel.NodeAttrNameAvs); "" != avs {
				// 填充属性视图角标 Display the database title on the block superscript https://github.com/siyuan-note/siyuan/issues/10545
				var avNames string
				if err = e.guard(func() (avErr error) { avNames, avErr = e.AvNames(e.Ctx, n.IALAttr(avmodel.NodeAttrNameAvs)); return }); err != nil {
					return ast.WalkStop
				}
				if "" != avNames {
					n.SetIALAttr(avmodel.NodeAttrViewNames, avNames)
				}
			}

			if "" != n.ID {
				// 填充块引计数
				if cnt := refCount[n.ID]; 0 < cnt {
					n.SetIALAttr("refcount", strconv.Itoa(cnt))
				}
			}

			if highlight && existKeywords {
				hitBlock := false
				for p := n.Parent; nil != p; p = p.Parent {
					if p.ID == id {
						hitBlock = true
						break
					}
				}
				if hitBlock {
					if ast.NodeCodeBlockCode == n.Type && !IsChartCodeBlockCode(n) {
						// 支持代码块搜索定位 https://github.com/siyuan-note/siyuan/issues/5520
						code := string(n.Tokens)
						markedCode := search.EncloseHighlighting(code, keywords, search.SearchMarkLeft, search.SearchMarkRight, e.CaseSensitive, false)
						if code != markedCode {
							n.Tokens = gulu.Str.ToBytes(markedCode)
							return ast.WalkContinue
						}
					} else if MarkReplaceSpan(n, &unlinks, keywords, search.MarkDataType, luteEngine, e.CaseSensitive) {
						return ast.WalkContinue
					}
				}
			}

			if existKeywords && id == n.ID {
				inlines := n.ChildrenByType(ast.NodeTextMark)
				for _, inline := range inlines {
					if inline.IsTextMarkType("inline-memo") && containsSubStr(inline.TextMarkInlineMemoContent, keywords) {
						// 支持行级备注搜索定位 https://github.com/siyuan-note/siyuan/issues/13465
						keywords = append(keywords, inline.TextMarkTextContent)
					}
				}
			}

			var processed bool
			if err = e.guard(func() (virtualErr error) {
				processed, virtualErr = e.Virtual(e.Ctx, n, &unlinks, virtualBlockRefKeywords, refCount, luteEngine)
				return
			}); err != nil {
				return ast.WalkStop
			}
			if processed {
				return ast.WalkContinue
			}
			return ast.WalkContinue
		})

		if err != nil {
			return
		}
		if err = e.Ctx.Err(); err != nil {
			return
		}
		for _, unlink := range unlinks {
			unlink.Unlink()
		}

		subTree.Root.AppendChild(n)
	}

	luteEngine.RenderOptions.NodeIndexStart = index
	if err = e.guard(func() (renderErr error) { dom, renderErr = e.Render(e.Ctx, subTree, luteEngine); return }); err != nil {
		return
	}

	if 1 > len(keywords) {
		keywords = []string{}
	}
	for i, keyword := range keywords {
		keyword = strings.TrimPrefix(keyword, "#")
		keyword = strings.TrimSuffix(keyword, "#")
		keywords[i] = keyword
	}
	keywords = gulu.Str.RemoveDuplicatedElem(keywords)

	if err = e.guard(func() error {
		return e.Recent(e.Ctx, Recent{ID: tree.Root.ID, Icon: tree.Root.IALAttr("icon"), Title: tree.Root.IALAttr("title"), Box: tree.Box, Path: tree.Path, HPath: tree.HPath})
	}); err != nil {
		return
	}
	return
}

func LoadNodesByStartEnd(ctx context.Context, tree *parse.Tree, startID, endID string, limit int) (nodes []*ast.Node, eof bool, err error) {
	if ctx == nil {
		return nil, false, errors.New("get_doc_context_unbound")
	}
	if err = ctx.Err(); err != nil {
		return nil, false, err
	}
	if tree == nil || tree.Root == nil {
		return nil, false, errors.New("get_doc_invalid_tree")
	}
	node := GetNodeInTree(tree, startID)
	if nil == node {
		return
	}
	nodes = append(nodes, node)
	for n := node.Next; nil != n; n = n.Next {
		if err = ctx.Err(); err != nil {
			return nil, false, err
		}
		if IsInFoldedHeading(n, nil) {
			continue
		}
		nodes = append(nodes, n)

		if n.ID == endID {
			if next := n.Next; nil == next {
				eof = true
			} else {
				eof = util2.IsDocIAL(n.Tokens) || util2.IsDocIAL(next.Tokens)
			}
			break
		}

		if len(nodes) >= limit {
			// 如果加载到指定数量的块则停止加载
			break
		}
	}
	return
}

func LoadNodesByMode(ctx context.Context, node *ast.Node, inputIndex, mode, size int, isDoc, isHeading bool) (nodes []*ast.Node, eof bool, err error) {
	if ctx == nil {
		return nil, false, errors.New("get_doc_context_unbound")
	}
	if err = ctx.Err(); err != nil {
		return nil, false, err
	}
	if node == nil {
		return nil, false, errors.New("get_doc_invalid_node")
	}
	if 2 == mode /* 向下 */ {
		next := node.Next
		if ast.NodeHeading == node.Type && "1" == node.IALAttr("fold") {
			// 标题展开时进行动态加载导致重复内容 https://github.com/siyuan-note/siyuan/issues/4671
			// 这里要考虑折叠标题是最后一个块的情况
			if children := HeadingChildren(node); 0 < len(children) {
				next = children[len(children)-1].Next
			}
		}
		if nil == next {
			eof = true
		} else {
			eof = util2.IsDocIAL(node.Tokens) || util2.IsDocIAL(next.Tokens)
		}
	}

	count := 0
	switch mode {
	case 0: // 仅加载当前 ID
		nodes = append(nodes, node)
		if isDoc {
			for n := node.Next; nil != n; n = n.Next {
				if err = ctx.Err(); err != nil {
					return nil, false, err
				}
				if IsInFoldedHeading(n, nil) {
					continue
				}
				nodes = append(nodes, n)
				if 1 > count {
					count++
				} else {
					count += CountBlockNodes(n)
				}
				if size < count {
					break
				}
			}
		} else if isHeading {
			level := node.HeadingLevel
			for n := node.Next; nil != n; n = n.Next {
				if err = ctx.Err(); err != nil {
					return nil, false, err
				}
				if IsInFoldedHeading(n, node) {
					// 大纲点击折叠标题跳转聚焦 https://github.com/siyuan-note/siyuan/issues/4920
					// 多级标题折叠后上级块引浮窗中未折叠 https://github.com/siyuan-note/siyuan/issues/4997
					continue
				}
				if ast.NodeHeading == n.Type {
					if n.HeadingLevel <= level {
						break
					}
				}
				nodes = append(nodes, n)
				count++
				if size < count {
					break
				}
			}
		}
	case 4: // Ctrl+End 跳转到末尾后向上加载
		for n := node; nil != n; n = n.Previous {
			if err = ctx.Err(); err != nil {
				return nil, false, err
			}
			if IsInFoldedHeading(n, nil) {
				continue
			}
			nodes = append([]*ast.Node{n}, nodes...)
			if 1 > count {
				count++
			} else {
				count += CountBlockNodes(n)
			}
			if size < count {
				break
			}
		}
		eof = true
	case 1: // 向上加载
		for n := node.Previous; /* 从上一个节点开始加载 */ nil != n; n = n.Previous {
			if err = ctx.Err(); err != nil {
				return nil, false, err
			}
			if IsInFoldedHeading(n, nil) {
				continue
			}
			nodes = append([]*ast.Node{n}, nodes...)
			if 1 > count {
				count++
			} else {
				count += CountBlockNodes(n)
			}
			if size < count {
				break
			}
		}
		eof = nil == node.Previous
	case 2: // 向下加载
		for n := node.Next; /* 从下一个节点开始加载 */ nil != n; n = n.Next {
			if err = ctx.Err(); err != nil {
				return nil, false, err
			}
			if IsInFoldedHeading(n, node) {
				continue
			}
			nodes = append(nodes, n)
			if 1 > count {
				count++
			} else {
				count += CountBlockNodes(n)
			}
			if size < count {
				break
			}
		}
	case 3: // 上下都加载
		for n := node; nil != n; n = n.Previous {
			if err = ctx.Err(); err != nil {
				return nil, false, err
			}
			if IsInFoldedHeading(n, nil) {
				continue
			}
			nodes = append([]*ast.Node{n}, nodes...)
			if 1 > count {
				count++
			} else {
				count += CountBlockNodes(n)
			}
			if 0 < inputIndex {
				if 1 < count {
					break // 滑块指示器加载
				}
			} else {
				if size < count {
					break
				}
			}
		}
		if size/2 < count {
			size = size / 2
		} else {
			size = size - count
		}
		count = 0
		for n := node.Next; nil != n; n = n.Next {
			if err = ctx.Err(); err != nil {
				return nil, false, err
			}
			if IsInFoldedHeading(n, nil) {
				continue
			}
			nodes = append(nodes, n)
			if 1 > count {
				count++
			} else {
				count += CountBlockNodes(n)
			}
			if 0 < inputIndex {
				if size < count {
					break
				}
			} else {
				if size < count {
					break
				}
			}
		}
	}
	return
}
