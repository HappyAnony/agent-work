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

package treenode

import (
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docview"
)

func MoveFoldHeading(updateNode, oldNode *ast.Node) { docview.MoveFoldHeading(updateNode, oldNode) }

func IsInFoldedHeading(node, currentHeading *ast.Node) bool {
	if nil == node {
		return false
	}

	heading := HeadingParent(node)
	if nil == heading {
		return false
	}
	if "1" == heading.IALAttr("heading-fold") || "1" == heading.IALAttr("fold") {
		return true
	}
	if heading == currentHeading {
		// node 就在当前标题层级下的话不递归继续查询，直接返回不折叠
		return false
	}
	return IsInFoldedHeading(heading, currentHeading)
}

func GetHeadingFold(nodes []*ast.Node) (ret []*ast.Node) { return docview.GetHeadingFold(nodes) }

func GetParentFoldedHeading(node *ast.Node) (parentFoldedHeading *ast.Node) {
	return docview.GetParentFoldedHeading(node)
}

func HeadingChildren(heading *ast.Node) (ret []*ast.Node) {
	start := heading.Next
	if nil == start {
		return
	}
	if ast.NodeKramdownBlockIAL == start.Type {
		start = start.Next // 跳过 heading 的 IAL
	}

	currentLevel := heading.HeadingLevel
	for n := start; nil != n; n = n.Next {
		if ast.NodeHeading == n.Type {
			if currentLevel >= n.HeadingLevel {
				break
			}
		}
		ret = append(ret, n)
	}
	return
}

func SuperBlockLastHeading(sb *ast.Node) *ast.Node {
	headings := sb.ChildrenByType(ast.NodeHeading)
	if 0 < len(headings) {
		return headings[len(headings)-1]
	}
	return nil
}

func HeadingParent(node *ast.Node) *ast.Node {
	if nil == node {
		return nil
	}

	currentLevel := 16
	if ast.NodeHeading == node.Type {
		currentLevel = node.HeadingLevel
	}

	for n := node.Previous; nil != n; n = n.Previous {
		if ast.NodeHeading == n.Type && n.HeadingLevel < currentLevel {
			return n
		}
	}
	return node.Parent
}

func HeadingLevel(node *ast.Node) int {
	if nil == node {
		return 0
	}

	for n := node; nil != n; n = n.Previous {
		if ast.NodeHeading == n.Type {
			return n.HeadingLevel
		}
	}
	return 0
}

func TopHeadingLevel(tree *parse.Tree) (ret int) {
	ret = 7
	for n := tree.Root.FirstChild; nil != n; n = n.Next {
		if ast.NodeHeading == n.Type {
			if ret > n.HeadingLevel {
				ret = n.HeadingLevel
			}
		}
	}
	if 7 == ret { // 没有出现过标题时
		ret = 0
	}
	return
}
