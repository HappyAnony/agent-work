// SPDX-License-Identifier: AGPL-3.0-or-later
// Original sql.getRefText/GetContainerText with explicit parser/block lookup.
package indexdb

import (
	"bytes"
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
)

type BlockReader func(context.Context, string) (BlockRecord, bool, error)

func ReferenceText(ctx context.Context, id string, parser *lute.Lute, read BlockReader) (string, error) {
	text, err := referenceText(ctx, id, parser, read, map[string]bool{})
	return strings.NewReplacer("__@mark__", "", "__mark@__", "").Replace(text), err
}
func referenceText(ctx context.Context, id string, parser *lute.Lute, read BlockReader, active map[string]bool) (string, error) {
	if parser == nil || read == nil {
		return "", ErrProjection
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if active[id] || len(active) > 128 {
		return "", ErrProjection
	}
	active[id] = true
	defer delete(active, id)
	b, ok, err := read(ctx, id)
	if err != nil {
		return "", err
	}
	if !ok {
		if strings.HasPrefix(id, "assets") {
			return id, nil
		}
		return "block not found", nil
	}
	if b.Name != "" {
		return b.Name, nil
	}
	switch b.Type {
	case "d":
		return b.Content, nil
	case "query_embed":
		return "Query Embed Block " + b.Markdown, nil
	case "av":
		return "Database " + b.Markdown, nil
	case "iframe":
		return "IFrame " + b.Markdown, nil
	case "tb":
		return "Thematic Break", nil
	case "video":
		return "Video " + b.Markdown, nil
	case "audio":
		return "Audio " + b.Markdown, nil
	}
	switch b.Type {
	case "l", "i", "s", "b":
		tree := parse.Parse("", []byte(b.Markdown), parser.ParseOptions)
		return ContainerText(ctx, tree.Root, func(ctx context.Context, id string) (string, error) {
			return referenceText(ctx, id, parser, read, active)
		})
	}
	return b.Content, nil
}
func ContainerText(ctx context.Context, container *ast.Node, read func(context.Context, string) (string, error)) (string, error) {
	if container == nil || read == nil {
		return "", ErrProjection
	}
	var leaf *ast.Node
	ast.Walk(container, func(n *ast.Node, enter bool) ast.WalkStatus {
		if !enter || n.IsMarker() {
			return ast.WalkContinue
		}
		if !n.IsContainerBlock() {
			leaf = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	if leaf == nil {
		return "", nil
	}
	var buf bytes.Buffer
	var err error
	ast.Walk(leaf, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = ctx.Err(); err != nil {
			return ast.WalkStop
		}
		if !enter {
			return ast.WalkContinue
		}
		switch n.Type {
		case ast.NodeText, ast.NodeLinkText, ast.NodeFileAnnotationRefText, ast.NodeCodeBlockCode, ast.NodeMathBlockContent:
			buf.Write(n.Tokens)
		case ast.NodeTextMark:
			buf.WriteString(n.Content())
		case ast.NodeBlockRef:
			if a := n.ChildByType(ast.NodeBlockRefText); a != nil {
				buf.WriteString(a.Text())
			} else if a = n.ChildByType(ast.NodeBlockRefDynamicText); a != nil {
				buf.WriteString(a.Text())
			} else {
				var s string
				s, err = read(ctx, n.TokensStr())
				if err != nil {
					return ast.WalkStop
				}
				buf.WriteString(s)
			}
			return ast.WalkSkipChildren
		}
		return ast.WalkContinue
	})
	return buf.String(), err
}
