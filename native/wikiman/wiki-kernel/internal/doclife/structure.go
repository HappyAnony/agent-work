// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"wiki-kernel/internal/docview"
)

// DocumentToHeading is the original AST conversion. Both inputs are detached;
// the caller owns history, source deletion, publication and derived indexes.
func DocumentToHeading(ctx context.Context, srcTree, targetTree *parse.Tree, targetID string, after bool, s TemplateBodyServices) error {
	if ctx == nil || srcTree == nil || srcTree.Root == nil || targetTree == nil || targetTree.Root == nil || s.NewID == nil || s.Now == nil {
		return ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if srcTree.ID == targetTree.ID {
		return ErrDependency
	}
	pivot := docview.GetNodeInTree(targetTree, targetID)
	if pivot == nil || pivot.Type == ast.NodeDocument {
		return ErrMissing
	}
	if ast.NodeListItem == pivot.Type {
		pivot = pivot.LastChild
	}

	pivotLevel := headingLevelAt(pivot)
	deltaLevel := pivotLevel - topHeadingLevel(srcTree) + 1
	level := pivotLevel
	if ast.NodeHeading == pivot.Type { // 平级插入
		children := docview.HeadingChildren(pivot)
		if after {
			if length := len(children); 0 < length {
				pivot = children[length-1]
			}
		}
	} else { // 子节点插入
		level++
		deltaLevel++
	}
	if 6 < level {
		level = 6
	}

	srcTree.Root.RemoveIALAttr("scroll") // Remove `scroll` attribute when converting the document to a heading https://github.com/siyuan-note/siyuan/issues/9297
	srcTree.Root.RemoveIALAttr("type")
	tagIAL := srcTree.Root.IALAttr("tags")
	tags := strings.Split(tagIAL, ",")
	srcTree.Root.RemoveIALAttr("tags")
	heading := &ast.Node{ID: srcTree.Root.ID, Type: ast.NodeHeading, HeadingLevel: level, KramdownIAL: srcTree.Root.KramdownIAL}
	heading.SetIALAttr("updated", s.Now().Format("20060102150405"))
	heading.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(srcTree.Root.IALAttr("title"))})
	heading.RemoveIALAttr("title")
	heading.Box, heading.Path = targetTree.Box, targetTree.Path
	if "" != tagIAL && 0 < len(tags) {
		// 带标签的文档块转换为标题块时将标签移动到标题块下方 https://github.com/siyuan-note/siyuan/issues/6550

		tagPara := templateParagraph(s)
		for i, tag := range tags {
			if e := ctx.Err(); e != nil {
				return e
			}
			if "" == tag {
				continue
			}

			tagPara.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "tag", TextMarkTextContent: tag})
			if i < len(tags)-1 {
				tagPara.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(" ")})
			}
		}
		if nil != tagPara.FirstChild {
			srcTree.Root.PrependChild(tagPara)
		}
	}

	var nodes []*ast.Node
	if after {
		for c := srcTree.Root.LastChild; nil != c; c = c.Previous {
			if e := ctx.Err(); e != nil {
				return e
			}
			nodes = append(nodes, c)
		}
	} else {
		for c := srcTree.Root.FirstChild; nil != c; c = c.Next {
			if e := ctx.Err(); e != nil {
				return e
			}
			nodes = append(nodes, c)
		}
	}

	if !after {
		pivot.InsertBefore(heading)
	}

	for _, n := range nodes {
		if e := ctx.Err(); e != nil {
			return e
		}
		if ast.NodeHeading == n.Type {
			n.HeadingLevel = n.HeadingLevel + deltaLevel
			if 6 < n.HeadingLevel {
				n.HeadingLevel = 6
			}
		}
		n.Box = targetTree.Box
		n.Path = targetTree.Path
		if after {
			pivot.InsertAfter(n)
		} else {
			pivot.InsertBefore(n)
		}
	}

	if after {
		pivot.InsertAfter(heading)
	}

	targetTree.Root.SetIALAttr("updated", s.Now().Format("20060102150405"))
	return ctx.Err()
}
func headingLevelAt(node *ast.Node) int {
	for n := node; n != nil; n = n.Previous {
		if n.Type == ast.NodeHeading {
			return n.HeadingLevel
		}
	}
	return 0
}
func topHeadingLevel(tree *parse.Tree) int {
	level := 7
	for n := tree.Root.FirstChild; n != nil; n = n.Next {
		if n.Type == ast.NodeHeading && n.HeadingLevel < level {
			level = n.HeadingLevel
		}
	}
	if level == 7 {
		return 0
	}
	return level
}

func HeadingToDocument(ctx context.Context, srcTree *parse.Tree, headingID, headingText string, dest Location, parser *lute.Lute, s TemplateBodyServices) (*parse.Tree, error) {
	if ctx == nil || srcTree == nil || srcTree.Root == nil || parser == nil || s.NewID == nil || s.Now == nil {
		return nil, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	headingNode := docview.GetNodeInTree(srcTree, headingID)
	if headingNode == nil || headingNode.Type != ast.NodeHeading {
		return nil, ErrMissing
	}
	// 折叠标题转换为文档时需要自动展开下方块 https://github.com/siyuan-note/siyuan/issues/2947
	children := docview.HeadingChildren(headingNode)
	for _, child := range children {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		ast.Walk(child, func(n *ast.Node, entering bool) ast.WalkStatus {
			if !entering {
				return ast.WalkContinue
			}

			n.RemoveIALAttr("heading-fold")
			n.RemoveIALAttr("fold")
			return ast.WalkContinue
		})
	}
	headingNode.RemoveIALAttr("fold")
	headingNode.RemoveIALAttr("heading-fold")

	newTree := &parse.Tree{Root: &ast.Node{Type: ast.NodeDocument, ID: headingID}, Context: &parse.Context{ParseOption: parser.ParseOptions}}
	for _, c := range children {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		newTree.Root.AppendChild(c)
	}
	newTree.ID = headingID
	newTree.Path = dest.Path
	newTree.HPath = dest.HPath
	headingNode.SetIALAttr("type", "doc")
	headingNode.SetIALAttr("id", headingID)
	headingNode.SetIALAttr("title", headingText)
	newTree.Root.KramdownIAL = headingNode.KramdownIAL

	topLevel := topHeadingLevel(newTree)
	for c := newTree.Root.FirstChild; nil != c; c = c.Next {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if ast.NodeHeading == c.Type {
			c.HeadingLevel = c.HeadingLevel - topLevel + 2
			if 6 < c.HeadingLevel {
				c.HeadingLevel = 6
			}
		}
	}

	headingNode.Unlink()
	srcTree.Root.SetIALAttr("updated", s.Now().Format("20060102150405"))
	if nil == srcTree.Root.FirstChild {
		srcTree.Root.AppendChild(templateParagraph(s))
	}
	newTree.Box = dest.Box
	if newTree.Root.FirstChild == nil {
		newTree.Root.AppendChild(templateParagraph(s))
	}
	newTree.Root.SetIALAttr("updated", s.Now().Format("20060102150405"))
	newTree.Root.Spec = "1"
	return newTree, ctx.Err()
}

func ListItemToDocument(ctx context.Context, source *parse.Tree, node *ast.Node, title string, dest Location, parser *lute.Lute, s TemplateBodyServices) (*parse.Tree, error) {
	if ctx == nil || source == nil || source.Root == nil || node == nil || node.Type != ast.NodeListItem ||
		parser == nil || s.NewID == nil || s.Now == nil {
		return nil, ErrDependency
	}
	root := &ast.Node{ID: node.ID, Type: ast.NodeDocument, Spec: "1"}
	for child := node.FirstChild; child != nil; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		next := child.Next
		if !child.IsMarker() {
			root.AppendChild(child)
		}
		child = next
	}
	if root.FirstChild == nil {
		root.AppendChild(templateParagraph(s))
	}
	root.KramdownIAL = node.KramdownIAL
	root.SetIALAttr("type", "doc")
	root.SetIALAttr("id", node.ID)
	root.SetIALAttr("title", title)
	root.RemoveIALAttr("fold")
	stamp := s.Now().Format("20060102150405")
	root.SetIALAttr("updated", stamp)
	parent := node.Parent
	node.Unlink()
	if parent != nil && parent.FirstChild == nil {
		parent.Unlink()
	}
	source.Root.SetIALAttr("updated", stamp)
	if source.Root.FirstChild == nil {
		source.Root.AppendChild(templateParagraph(s))
	}
	return &parse.Tree{ID: node.ID, Box: dest.Box, Path: dest.Path, HPath: dest.HPath, Root: root,
		Context: &parse.Context{ParseOption: parser.ParseOptions}}, ctx.Err()
}
