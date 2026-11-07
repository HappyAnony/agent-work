// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/blockview"

	"wiki-kernel/internal/editcore"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
)

func renderOutline(heading *ast.Node, luteEngine *lute.Lute) string {
	return blockview.Outline(heading, luteEngine)
}

func renderBlockText(n *ast.Node, exclude []string, removeLineBreak bool) string {
	ret, _ := legacyPositionText(context.Background(), n, exclude, removeLineBreak)
	return ret
}

func fillBlockRefCount(nodes []*ast.Node) {
	_ = fillBlockRefCountWithQuery(nodes, func(ids []string) (map[string]int, error) { return sql.QueryRefCount(ids), nil })
}

func fillBlockRefCountWithQuery(nodes []*ast.Node, query func([]string) (map[string]int, error), contexts ...context.Context) error {
	return editcore.
		FillBlockRefCountWithQuery(nodes, query,
			contexts...)
}

func renderBlockDOMByNodes(nodes []*ast.Node, luteEngine *lute.Lute) string {
	ret, _ := renderBlockDOMWithNames(nodes, luteEngine, func(ids string) (string, error) { return getAvNames(ids), nil })
	return ret
}

func renderBlockDOMWithNames(nodes []*ast.Node, parser *lute.Lute, names func(string) (string, error), contexts ...context.Context) (string, error) {
	return editcore.
		RenderBlockDOMWithNames(nodes, parser,
			names,
			contexts...)
}

func resolveEmbedR(n *ast.Node, blockEmbedMode int, luteEngine *lute.Lute, resolved *[]string, depth *int, ports ...*exportReferenceRenderer) {
	if !exportRenderReady(ports) {
		return
	}
	var children []*ast.Node
	if ast.NodeHeading == n.Type {
		children = append(children, n)
		children = append(children, treenode.HeadingChildren(n)...)
	} else if ast.NodeDocument == n.Type {
		for c := n.FirstChild; nil != c; c = c.Next {
			children = append(children, c)
		}
	} else {
		children = append(children, n)
	}

	*depth++
	if 7 < *depth {
		return
	}

	for _, child := range children {
		var unlinks []*ast.Node

		parentHeadingLevel := 0
		for prev := child; nil != prev; prev = prev.Previous {
			if ast.NodeHeading == prev.Type {
				parentHeadingLevel = prev.HeadingLevel
				break
			}
		}

		ast.Walk(child, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !exportRenderReady(ports) {
				return ast.WalkStop
			}
			if !entering || !n.IsBlock() {
				return ast.WalkContinue
			}

			if ast.NodeBlockQueryEmbed == n.Type {
				if gulu.Str.Contains(n.ID, *resolved) {
					return ast.WalkContinue
				}
				*resolved = append(*resolved, n.ID)

				script := n.ChildByType(ast.NodeBlockQueryEmbedScript)
				if script == nil && len(ports) > 0 {
					exportRenderFailure(ports, errors.New("export_embed_script_missing"))
					return ast.WalkStop
				}
				stmt := script.TokensStr()
				stmt = html.UnescapeString(stmt)
				stmt = strings.ReplaceAll(stmt, editor.IALValEscNewLine, "\n")
				sqlBlocks := exportEmbedQuery(stmt, ports)
				for _, sqlBlock := range sqlBlocks {
					if "query_embed" == sqlBlock.Type {
						continue
					}

					subTree, loadErr := exportEmbedLoad(sqlBlock.ID, ports)
					if loadErr != nil && len(ports) > 0 {
						return ast.WalkStop
					}
					if nil == subTree {
						continue
					}

					var md string
					if "d" == sqlBlock.Type {
						if 0 == blockEmbedMode {
							// 嵌入块中出现了大于等于上方非嵌入块的标题时需要降低嵌入块中的标题级别
							// Improve export of heading levels in embedded blocks https://github.com/siyuan-note/siyuan/issues/12233 https://github.com/siyuan-note/siyuan/issues/12741
							embedTopLevel := 0
							ast.Walk(subTree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
								if !entering || ast.NodeHeading != n.Type {
									return ast.WalkContinue
								}

								embedTopLevel = n.HeadingLevel
								if parentHeadingLevel >= embedTopLevel {
									n.HeadingLevel += parentHeadingLevel - embedTopLevel + 1
									if 6 < n.HeadingLevel {
										n.HeadingLevel = 6
									}
								}
								return ast.WalkContinue
							})
						}

						md = exportEmbedFormat(subTree.Root, luteEngine, ports)
					} else if "h" == sqlBlock.Type {
						h := treenode.GetNodeInTree(subTree, sqlBlock.ID)
						if h == nil && len(ports) > 0 {
							exportRenderFailure(ports, errors.New("export_embed_node_missing"))
							return ast.WalkStop
						}
						var hChildren []*ast.Node

						// 从嵌入块的 IAL 属性中解析 custom-heading-mode，使用全局配置作为默认值
						blockHeadingMode := exportEmbedHeadingMode(ports)
						if customHeadingMode := n.IALAttr("custom-heading-mode"); "" != customHeadingMode {
							if mode, err := strconv.Atoi(customHeadingMode); nil == err && (mode == 0 || mode == 1 || mode == 2) {
								blockHeadingMode = mode
							}
						}

						// 根据 blockHeadingMode 处理标题块的显示
						// blockHeadingMode: 0=显示标题与下方的块，1=仅显示标题，2=仅显示标题下方的块
						if 1 == blockHeadingMode {
							// 仅显示标题
							hChildren = append(hChildren, h)
						} else if 2 == blockHeadingMode {
							// 仅显示标题下方的块（默认行为）
							if "1" != h.IALAttr("fold") {
								children := treenode.HeadingChildren(h)
								for _, c := range children {
									if "1" == c.IALAttr("heading-fold") {
										// 嵌入块包含折叠标题时不应该显示其下方块 https://github.com/siyuan-note/siyuan/issues/4765
										continue
									}
									hChildren = append(hChildren, c)
								}
							}
						} else {
							// 0: 显示标题与下方的块
							hChildren = append(hChildren, h)
							hChildren = append(hChildren, treenode.HeadingChildren(h)...)
						}
						if 0 == blockEmbedMode {
							embedTopLevel := 0
							for _, hChild := range hChildren {
								if ast.NodeHeading == hChild.Type {
									embedTopLevel = hChild.HeadingLevel
									break
								}
							}
							if parentHeadingLevel >= embedTopLevel {
								for _, hChild := range hChildren {
									if ast.NodeHeading == hChild.Type {
										hChild.HeadingLevel += parentHeadingLevel - embedTopLevel + 1
										if 6 < hChild.HeadingLevel {
											hChild.HeadingLevel = 6
										}
									}
								}
							}
						}

						mdBuf := &bytes.Buffer{}
						for _, hChild := range hChildren {
							md = exportEmbedFormat(hChild, luteEngine, ports)
							mdBuf.WriteString(md)
							mdBuf.WriteString("\n\n")
						}
						md = mdBuf.String()
					} else {
						node := treenode.GetNodeInTree(subTree, sqlBlock.ID)
						if node == nil && len(ports) > 0 {
							exportRenderFailure(ports, errors.New("export_embed_node_missing"))
							return ast.WalkStop
						}
						md = exportEmbedFormat(node, luteEngine, ports)
					}

					buf := &bytes.Buffer{}
					lines := strings.Split(md, "\n")
					for i, line := range lines {
						if 0 == blockEmbedMode { // 使用原始文本
							buf.WriteString(line)
						} else { // 使用引述块
							buf.WriteString("> " + line)
						}
						if i < len(lines)-1 {
							buf.WriteString("\n")
						}
					}
					buf.WriteString("\n\n")

					subTree = parse.Parse("", buf.Bytes(), luteEngine.ParseOptions)
					var inserts []*ast.Node
					for subNode := subTree.Root.FirstChild; nil != subNode; subNode = subNode.Next {
						if ast.NodeKramdownBlockIAL != subNode.Type {
							inserts = append(inserts, subNode)
						}
					}
					if 2 < len(n.KramdownIAL) && 0 < len(inserts) {
						if bookmark := n.IALAttr("bookmark"); "" != bookmark {
							inserts[0].SetIALAttr("bookmark", bookmark)
						}
						if name := n.IALAttr("name"); "" != name {
							inserts[0].SetIALAttr("name", name)
						}
						if alias := n.IALAttr("alias"); "" != alias {
							inserts[0].SetIALAttr("alias", alias)
						}
						if memo := n.IALAttr("memo"); "" != memo {
							inserts[0].SetIALAttr("memo", memo)
						}
					}
					for _, insert := range inserts {
						n.InsertBefore(insert)

						if gulu.Str.Contains(sqlBlock.ID, *resolved) {
							return ast.WalkContinue
						}

						resolveEmbedR(insert, blockEmbedMode, luteEngine, resolved, depth, ports...)
					}
				}
				unlinks = append(unlinks, n)
				return ast.WalkSkipChildren
			}
			return ast.WalkContinue
		})
		for _, unlink := range unlinks {
			unlink.Unlink()
		}
	}
	return
}

func renderBlockMarkdownR(id string, rendered *[]string, ports ...*exportReferenceRenderer) (ret []*ast.Node) {
	if !exportRenderReady(ports) {
		return
	}
	if gulu.Str.Contains(id, *rendered) {
		return
	}
	*rendered = append(*rendered, id)

	b := exportRenderLookup(id, ports)
	if nil == b {
		if len(ports) > 0 {
			exportRenderFailure(ports, ErrBlockNotFound)
		}
		return
	}

	var err error
	var t *parse.Tree
	if t, err = exportEmbedLoad(b.ID, ports); err != nil {
		return
	}
	node := treenode.GetNodeInTree(t, b.ID)
	if nil == node {
		exportRenderFailure(ports, errors.New("export_recursive_node_missing"))
		return
	}

	var children []*ast.Node
	if ast.NodeHeading == node.Type {
		children = append(children, node)
		children = append(children, treenode.HeadingChildren(node)...)
	} else if ast.NodeDocument == node.Type {
		for c := node.FirstChild; nil != c; c = c.Next {
			children = append(children, c)
		}
	} else {
		children = append(children, node)
	}

	for _, child := range children {
		if !exportRenderReady(ports) {
			return nil
		}
		var unlinks, inserts []*ast.Node
		ast.Walk(child, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !exportRenderReady(ports) {
				return ast.WalkStop
			}
			if !entering || !n.IsBlock() {
				return ast.WalkContinue
			}

			if ast.NodeBlockQueryEmbed == n.Type {
				script := n.ChildByType(ast.NodeBlockQueryEmbedScript)
				if script == nil && len(ports) > 0 {
					exportRenderFailure(ports, errors.New("export_recursive_script_missing"))
					return ast.WalkStop
				}
				stmt := script.TokensStr()
				stmt = html.UnescapeString(stmt)
				stmt = strings.ReplaceAll(stmt, editor.IALValEscNewLine, "\n")
				sqlBlocks := exportEmbedQuery(stmt, ports)
				for _, sqlBlock := range sqlBlocks {
					subNodes := renderBlockMarkdownR(sqlBlock.ID, rendered, ports...)
					for _, subNode := range subNodes {
						inserts = append(inserts, subNode)
					}
				}
				unlinks = append(unlinks, n)
				return ast.WalkSkipChildren
			}
			return ast.WalkContinue
		})
		for _, n := range unlinks {
			n.Unlink()
		}

		if ast.NodeBlockQueryEmbed != child.Type {
			ret = append(ret, child)
		} else {
			for _, n := range inserts {
				ret = append(ret, n)
			}
		}

	}
	if !exportRenderReady(ports) {
		return nil
	}
	return
}
