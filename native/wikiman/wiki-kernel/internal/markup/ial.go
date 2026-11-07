// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package markup

import (
	"bytes"
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
)

func AddBlockIALNodes(ctx context.Context, tree *parse.Tree, removeUpdated bool) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	var blocks []*ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if !entering || !n.IsBlock() {
			return ast.WalkContinue
		}

		if ast.NodeBlockQueryEmbed == n.Type {
			if script := n.ChildByType(ast.NodeBlockQueryEmbedScript); nil != script {
				script.Tokens = bytes.ReplaceAll(script.Tokens, []byte("\n"), []byte(" "))
			}
		} else if ast.NodeHTMLBlock == n.Type {
			n.Tokens = bytes.TrimSpace(n.Tokens)
			// 使用 <div> 包裹，否则后续解析时会识别为行级 HTML https://github.com/siyuan-note/siyuan/issues/4244
			if !bytes.HasPrefix(n.Tokens, []byte("<div>")) {
				n.Tokens = append([]byte("<div>\n"), n.Tokens...)
			}
			if !bytes.HasSuffix(n.Tokens, []byte("</div>")) {
				n.Tokens = append(n.Tokens, []byte("\n</div>")...)
			}
		}

		if removeUpdated {
			n.RemoveIALAttr("updated")
		}
		if 0 < len(n.KramdownIAL) {
			blocks = append(blocks, n)
		}
		return ast.WalkContinue
	})
	for _, block := range blocks {
		block.InsertAfter(&ast.Node{Type: ast.NodeKramdownBlockIAL, Tokens: parse.IAL2Tokens(block.KramdownIAL)})
	}
	return ctx.Err()
}
