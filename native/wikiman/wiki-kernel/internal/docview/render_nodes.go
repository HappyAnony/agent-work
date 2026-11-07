// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"strconv"
	"strings"
)

func RenderNodes(ctx context.Context, nodes []*ast.Node, parser *lute.Lute, names func(context.Context, string) (string, error)) (string, error) {
	var err error
	tree := &parse.Tree{Root: &ast.Node{Type: ast.NodeDocument}, Context: &parse.Context{ParseOption: parser.ParseOptions}}
	renderer := render.NewProtyleRenderer(tree, parser.RenderOptions)
	for _, node := range nodes {
		ast.Walk(node, func(n *ast.Node, enter bool) ast.WalkStatus {
			if err = ctx.Err(); err != nil {
				return ast.WalkStop
			}
			if enter && n.IsBlock() {
				if ids := n.IALAttr("custom-avs"); ids != "" {
					var value string
					value, err = names(ctx, ids)
					if err != nil {
						return ast.WalkStop
					}
					if value != "" {
						n.SetIALAttr("av-names", value)
					}
				}
			}
			return renderer.RendererFuncs[n.Type](n, enter)
		})
		if err != nil {
			return "", err
		}
	}
	text := strings.TrimSpace(renderer.Writer.String())
	if strings.HasPrefix(text, "<li") {
		text = "<ul>" + text + "</ul>"
	}
	return text, ctx.Err()
}

func FillReferenceCounts(ctx context.Context, nodes []*ast.Node, query func(context.Context, []string) (map[string]int, error)) error {
	ids := []string{}
	for _, n := range nodes {
		ast.Walk(n, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if enter && n.IsBlock() {
				ids = append(ids, n.ID)
			}
			return ast.WalkContinue
		})
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	counts, e := query(ctx, ids)
	if e != nil {
		return e
	}
	for _, n := range nodes {
		ast.Walk(n, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if enter && n.IsBlock() && counts[n.ID] > 0 {
				n.SetIALAttr("refcount", strconv.Itoa(counts[n.ID]))
			}
			return ast.WalkContinue
		})
	}
	return ctx.Err()
}

func AVNames(ctx context.Context, ids string, lookup func(context.Context, string) (string, error), fallback func(context.Context) (string, error)) (string, error) {
	values := []string{}
	for _, id := range strings.Split(ids, ",") {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if id == "" {
			continue
		}
		name, err := lookup(ctx, id)
		if err != nil {
			return "", err
		}
		if name == "" {
			name, err = fallback(ctx)
			if err != nil {
				return "", err
			}
		}
		values = append(values, `<span data-av-id="`+id+`" data-popover-url="/api/av/getMirrorDatabaseBlocks" class="popover__block">`+name+`</span>`)
	}
	return strings.Join(values, "&nbsp;"), ctx.Err()
}
