// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"bytes"
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"wiki-kernel/internal/markup"
)

// Format preserves the original two-pass typography algorithm and document IAL.
// Inputs are detached; the parser's render settings remain local to this call.
func Format(ctx context.Context, tree *parse.Tree, parser *lute.Lute, s TemplateBodyServices) (*parse.Tree, error) {
	if ctx == nil || tree == nil || tree.Root == nil || parser == nil {
		return nil, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	p := *parser
	options := *parser.RenderOptions
	p.RenderOptions = &options
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if !enter {
			return ast.WalkContinue
		}
		switch n.Type {
		case ast.NodeTextMark:
			p.MergeSameTextMark(n)
		case ast.NodeCodeBlockCode:
			n.Tokens = bytes.ReplaceAll(n.Tokens, []byte(editor.Zwj+"```"), []byte("```"))
			n.Tokens = bytes.ReplaceAll(n.Tokens, []byte("```"), []byte(editor.Zwj+"```"))
		}
		return ast.WalkContinue
	})
	ial := tree.Root.KramdownIAL
	if e := markup.AddBlockIALNodes(ctx, tree, false); e != nil {
		return nil, e
	}
	md := render.NewFormatRenderer(tree, p.RenderOptions).Render()
	next := parse.Parse("", md, p.ParseOptions)
	if _, _, _, e := NormalizeCreationTemplate(ctx, next, s); e != nil {
		return nil, e
	}
	next.Root.Spec = "1"
	p.SetAutoSpace(true)
	md = render.NewFormatRenderer(next, p.RenderOptions).Render()
	next = parse.Parse("", md, p.ParseOptions)
	if _, _, _, e := NormalizeCreationTemplate(ctx, next, s); e != nil {
		return nil, e
	}
	next.Root.Spec = "1"
	next.Root.ID = tree.ID
	next.Root.KramdownIAL = ial
	next.ID = tree.ID
	next.Path = tree.Path
	next.HPath = tree.HPath
	next.Box = tree.Box
	return next, ctx.Err()
}
