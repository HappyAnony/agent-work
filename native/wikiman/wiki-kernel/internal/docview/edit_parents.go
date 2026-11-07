// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	ast "github.com/88250/lute/ast"
)

func ParentNodesWithHeadings(node *ast.Node, warning ...func(string, ...any)) (parents []*ast.Node) {
	const maxDepth = 255
	i := 0
	for n := node; nil != n; n = n.Parent {
		parent := n.Parent
		if maxDepth < i {
			if len(warning) > 0 && warning[0] != nil {
				warning[0]("parent nodes of node [%s] is too deep", node.ID)
			}
			return
		}
		i++

		if nil == parent {
			return
		}

		// 标题下方块编辑后刷新标题块更新时间
		// The heading block update time is refreshed after editing the blocks under the heading https://github.com/siyuan-note/siyuan/issues/11374
		parentHeadingLevel := 7
		if ast.NodeHeading == n.Type {
			parentHeadingLevel = n.HeadingLevel
		}
		for prev := n.Previous; nil != prev; prev = prev.Previous {
			if ast.NodeHeading == prev.Type {
				if prev.HeadingLevel >= parentHeadingLevel {
					break
				}

				parents = append(parents, prev)
				parentHeadingLevel = prev.HeadingLevel
			}
		}

		parents = append(parents, parent)
		if ast.NodeDocument == parent.Type {
			return
		}
	}
	return
}
