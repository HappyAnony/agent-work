// SPDX-License-Identifier: AGPL-3.0-or-later
package aiflow

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"wiki-kernel/internal/nativeparser"
)

// Trees and parser are owned by the caller. Document requests expand direct
// children in original order; missing IDs skip and repeated IDs remain repeated.
func Blocks(ctx context.Context, ids []string, trees map[string]*parse.Tree, markdown nativeparser.Markdown) (string, error) {
	p := nativeparser.NewLute(markdown)
	var buf strings.Builder
	for _, id := range ids {
		if e := ctx.Err(); e != nil {
			return "", e
		}
		tree := trees[id]
		if tree == nil || tree.Root == nil {
			continue
		}
		var node *ast.Node
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if enter && n.ID == id {
				node = n
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
		if node == nil {
			continue
		}
		nodes := []*ast.Node{node}
		if node.Type == ast.NodeDocument {
			nodes = nil
			for n := node.FirstChild; n != nil; n = n.Next {
				nodes = append(nodes, n)
			}
		}
		for _, n := range nodes {
			if e := ctx.Err(); e != nil {
				return "", e
			}
			md, e := lute.ProtyleExportMdNodeSync(n, p.ParseOptions, p.RenderOptions)
			if e != nil {
				return "", ErrConfig
			}
			buf.WriteString(md)
			buf.WriteString("\n\n")
		}
	}
	return buf.String(), ctx.Err()
}
