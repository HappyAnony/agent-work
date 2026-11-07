// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	"errors"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
)

// FormatHTMLPaste retains the original Markdown round-trip and normalization.
// A caller that owns clipboard files can explicitly copy their link destinations.
func FormatHTMLPaste(engine *lute.Lute, tree *parse.Tree, withMath bool, guard func() error, copyAsset func(string) (string, error)) (string, error) {
	if engine == nil || tree == nil || tree.Root == nil || guard == nil {
		return "", errors.New("html_paste_tree_unbound")
	}
	formatter := render.NewFormatRenderer(tree, engine.RenderOptions)
	for kind, fn := range engine.HTML2MdRendererFuncs {
		formatter.ExtRendererFuncs[kind] = fn
	}
	markdown := formatter.Render()
	if err := guard(); err != nil {
		return "", err
	}
	if withMath {
		engine.SetInlineMath(true)
	}
	tree = parse.Parse("", markdown, engine.ParseOptions)
	if tree == nil || tree.Root == nil {
		return "", errors.New("html_paste_tree_unbound")
	}
	var failed error
	walk := func(fn func(*ast.Node, bool) ast.WalkStatus) {
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if failed = guard(); failed != nil {
				return ast.WalkStop
			}
			return fn(n, enter)
		})
	}
	var unlinks []*ast.Node
	walk(func(n *ast.Node, enter bool) ast.WalkStatus {
		if !enter {
			return ast.WalkContinue
		}
		if n.Type == ast.NodeListItem && n.FirstChild == nil {
			node := NewParagraph("")
			n.AppendChild(node)
			n.SetIALAttr("updated", node.ID[:14])
			return ast.WalkSkipChildren
		}
		if n.Type == ast.NodeBlockquote && n.FirstChild != nil && n.FirstChild.Next == nil {
			unlinks = append(unlinks, n)
		}
		return ast.WalkContinue
	})
	if failed != nil {
		return "", failed
	}
	for _, node := range unlinks {
		node.Unlink()
	}
	// Preserve the original single-cell spreadsheet/table-to-paragraph rule.
	if table := tree.Root.FirstChild; table != nil && table.Type == ast.NodeTable && (table.Next == nil || table.Next.Type == ast.NodeKramdownBlockIAL && table.Next.Next == nil) {
		if head := table.FirstChild; head != nil && head.Type == ast.NodeTableHead && head.Next == nil && head.FirstChild != nil && head.FirstChild.Next == nil {
			row := head.FirstChild
			if row.FirstChild != nil && row.FirstChild.Next == nil {
				cell := row.FirstChild
				paragraph := NewParagraph("")
				var children []*ast.Node
				for c := cell.FirstChild; c != nil; c = c.Next {
					children = append(children, c)
				}
				for _, c := range children {
					paragraph.AppendChild(c)
				}
				table.Unlink()
				tree.Root.PrependChild(paragraph)
			}
		}
	}
	if copyAsset != nil {
		walk(func(n *ast.Node, enter bool) ast.WalkStatus {
			if !enter || n.Type != ast.NodeLinkDest {
				return ast.WalkContinue
			}
			name, err := copyAsset(n.TokensStr())
			if err != nil {
				failed = err
				return ast.WalkStop
			}
			if name != "" {
				n.Tokens = []byte("assets/" + name)
			}
			return ast.WalkContinue
		})
		if failed != nil {
			return "", failed
		}
	}
	parse.TextMarks2Inlines(tree)
	parse.NestedInlines2FlattedSpansHybrid(tree, false)
	if err := guard(); err != nil {
		return "", err
	}
	renderer := render.NewProtyleRenderer(tree, engine.RenderOptions)
	return string(renderer.Render()), guard()
}
