// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"bytes"
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"strings"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/markup"
	"wiki-kernel/internal/textprefs"
)

func TemplateFilename(name string) string {
	return textprefs.TruncateLenFileName(textprefs.FilterFileName(name) + ".md")
}

// The caller supplies a detached, owner-loaded source tree and parser. This is
// the original document/block selection and template export, with no source IO.
func TemplateMarkdown(ctx context.Context, tree *parse.Tree, id string, p *lute.Lute) ([]byte, error) {
	if ctx == nil || tree == nil || tree.Root == nil || p == nil {
		return nil, ErrDependency
	}
	n := docview.GetNodeInTree(tree, id)
	if n == nil {
		return nil, ErrMissing
	}
	if n.Type != ast.NodeDocument {
		nodes := []*ast.Node{n}
		if n.Type == ast.NodeHeading {
			nodes = append(nodes, docview.HeadingChildren(n)...)
		}
		old := tree.Root
		tree = parse.Parse("", []byte(""), p.ParseOptions)
		first := tree.Root.FirstChild
		for _, node := range nodes {
			first.InsertBefore(node)
		}
		tree.Root.KramdownIAL = old.KramdownIAL
	}
	if e := markup.AddBlockIALNodes(ctx, tree, true); e != nil {
		return nil, e
	}
	var unlinks []*ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if !enter {
			return ast.WalkContinue
		}
		switch n.Type {
		case ast.NodeCodeBlockCode, ast.NodeCodeSpanContent, ast.NodeBlockQueryEmbedScript:
			n.Tokens = bytes.ReplaceAll(n.Tokens, []byte("&quot;"), []byte("\""))
		case ast.NodeTextMark:
			if n.IsTextMarkType("code") {
				n.TextMarkTextContent = strings.ReplaceAll(n.TextMarkTextContent, "&quot;", "\"")
			}
		}
		if n.Type == ast.NodeCodeBlockFenceInfoMarker && (string(n.CodeBlockInfo) == "template" || string(n.CodeBlockInfo) == "siyuan-template") {
			if n.Parent == nil || n.Next == nil {
				return ast.WalkContinue
			}
			unlinks = append(unlinks, n.Parent)
			para := &ast.Node{Type: ast.NodeParagraph, ID: n.Parent.ID}
			para.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: n.Next.Tokens})
			n.Parent.InsertBefore(para)
		}
		return ast.WalkContinue
	})
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	for _, n := range unlinks {
		n.Unlink()
	}
	md := render.NewFormatRenderer(tree, p.RenderOptions).Render()
	if len(tree.Root.KramdownIAL) > 0 {
		tree.Root.RemoveIALAttr("id")
		tree.Root.KramdownIAL = append([][]string{{"id", tree.Root.ID}}, tree.Root.KramdownIAL...)
		md = append(md, '\n')
		md = append(md, parse.IAL2Tokens(tree.Root.KramdownIAL)...)
	}
	return md, ctx.Err()
}
