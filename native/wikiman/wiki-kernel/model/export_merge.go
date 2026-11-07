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
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/filesys"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func mergeSubDocs(rootTree *parse.Tree, ports ...*exportMergeServices) (ret *parse.Tree, err error) {
	if err = exportMergeGuard(ports); err != nil {
		return nil, err
	}
	if rootTree == nil || rootTree.Root == nil {
		return nil, errors.New("export_merge_tree_unbound")
	}
	ret = rootTree
	rootBlock := &Block{Box: rootTree.Box, ID: rootTree.ID, Path: rootTree.Path, HPath: rootTree.HPath}
	if err = buildBlockChildren(rootBlock, ports...); err != nil {
		return
	}

	insertPoint := rootTree.Root.LastChild

	// 跳过空段落插入点，向上寻找非空段落
	for ; nil != insertPoint && ast.NodeParagraph == insertPoint.Type; insertPoint = insertPoint.Previous {
		if nil != insertPoint.FirstChild {
			break
		}
	}

	// 导出空文档 Word 和 PDF 时合并子文档失败 https://github.com/siyuan-note/siyuan/issues/7429
	if nil == insertPoint {
		// 如果找不到非空段落，则使用第一个段落作为插入点
		insertPoint = rootTree.Root.FirstChild
		if nil == insertPoint {
			// 如果文档为空，则创建一个空段落作为插入点
			insertPoint = treenode.NewParagraph("")
			rootTree.Root.AppendChild(insertPoint)
		}
	}

	for {
		i := 0
		if err = walkBlock(insertPoint, rootBlock, i, ports...); err != nil {
			return
		}
		if nil == rootBlock.Children {
			break
		}
	}

	if ast.NodeParagraph == insertPoint.Type && nil == insertPoint.FirstChild {
		// 删除空段落
		// Ignore the last empty paragraph block when exporting merged sub-documents https://github.com/siyuan-note/siyuan/issues/15028
		insertPoint.Unlink()
	}
	return
}

func walkBlock(insertPoint *ast.Node, block *Block, level int, ports ...*exportMergeServices) (err error) {
	if err = exportMergeGuard(ports); err != nil {
		return
	}
	level++
	for i := len(block.Children) - 1; i >= 0; i-- {
		c := block.Children[i]
		if err = walkBlock(insertPoint, c, level, ports...); err != nil {
			return
		}

		nodes, loadErr := loadTreeNodes(c.Box, c.Path, level, ports...)
		if nil != loadErr {
			err = loadErr
			return
		}

		lastIndex := len(nodes) - 1
		for j := lastIndex; -1 < j; j-- {
			node := nodes[j]
			if j == lastIndex && ast.NodeParagraph == node.Type && nil == node.FirstChild {
				// 跳过最后一个空段落块
				// Ignore the last empty paragraph block when exporting merged sub-documents https://github.com/siyuan-note/siyuan/issues/15028
				continue
			}
			insertPoint.InsertAfter(node)
		}
	}
	block.Children = nil
	return
}

func loadTreeNodes(box string, p string, level int, ports ...*exportMergeServices) (ret []*ast.Node, err error) {
	if err = exportMergeGuard(ports); err != nil {
		return
	}
	var tree *parse.Tree
	if len(ports) > 0 {
		tree, err = ports[0].load(box, p)
	} else {
		tree, err = filesys.LoadTree(box, p, NewLute())
	}
	if err != nil {
		return
	}
	if tree == nil || tree.Root == nil {
		return nil, errors.New("export_merge_tree_unbound")
	}
	if len(ports) > 0 && (tree.Box != box || tree.Path != p || tree.ID != util.GetTreeID(p)) {
		return nil, errors.New("export_merge_source_mismatch")
	}
	return exportMergeNodes(tree, level), nil
}
func exportMergeNodes(tree *parse.Tree, level int) (ret []*ast.Node) {
	hLevel := level
	if 6 < level {
		hLevel = 6
	}

	heading := &ast.Node{ID: tree.Root.ID, Type: ast.NodeHeading, HeadingLevel: hLevel}
	heading.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(tree.Root.IALAttr("title"))})
	tree.Root.PrependChild(heading)
	for c := tree.Root.FirstChild; nil != c; c = c.Next {
		ret = append(ret, c)
	}
	return
}

func buildBlockChildren(block *Block, ports ...*exportMergeServices) (err error) {
	if err = exportMergeGuard(ports); err != nil {
		return
	}
	var files []*File
	if len(ports) > 0 {
		files, err = ports[0].list(block.Box, block.Path)
	} else {
		files, _, err = ListDocTree(block.Box, block.Path, util.SortModeUnassigned, false, false, Conf.FileTree.MaxListCount)
	}
	if err != nil {
		return
	}

	for _, f := range files {
		childBlock := &Block{Box: block.Box, ID: f.ID, Path: f.Path}
		block.Children = append(block.Children, childBlock)
	}

	for _, c := range block.Children {
		if err = buildBlockChildren(c, ports...); err != nil {
			return
		}
	}
	return
}
