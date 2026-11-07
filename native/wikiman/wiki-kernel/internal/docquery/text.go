// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model/render.go renderBlockText with explicit static-content reader.
package docquery

import (
	"context"
	"github.com/88250/lute/ast"
	"slices"
	"strings"
)

func BlockText(ctx context.Context, node *ast.Node, exclude []string, removeLineBreak bool, read func(context.Context, *ast.Node, []string) (string, error)) (string, error) {
	if ctx == nil || read == nil {
		return "", ErrPositionDependency
	}
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if node == nil {
		return "", nil
	}
	ret, e := read(ctx, node, slices.Clone(exclude))
	if e != nil {
		return "", e
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	ret = strings.TrimSpace(ret)
	if removeLineBreak {
		ret = strings.ReplaceAll(ret, "\n", "")
	}
	ret = cleanBreadcrumbHTML(ret)
	ret = strings.TrimSpace(ret)
	if ret == "" {
		var b strings.Builder
		ast.Walk(node, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if !enter || n.Type != ast.NodeImage {
				return ast.WalkContinue
			}
			if title := n.ChildByType(ast.NodeLinkTitle); title != nil {
				b.Write(title.Tokens)
			} else if alt := n.ChildByType(ast.NodeLinkText); alt != nil && len(alt.Tokens) > 0 {
				b.Write(alt.Tokens)
			} else {
				b.WriteString("image")
			}
			return ast.WalkContinue
		})
		ret = b.String()
	}
	return ret, ctx.Err()
}
