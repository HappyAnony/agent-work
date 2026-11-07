// SPDX-License-Identifier: AGPL-3.0-or-later
package docview

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"github.com/siyuan-note/dataparser"
	"strings"
	"wiki-kernel/search"
)

type HistoryResult struct {
	ID, RootID, Title, Content string
	Large                      bool
}

// RenderHistory uses original detached history bytes, without repair or writes.
func RenderHistory(ctx context.Context, raw []byte, keyword string, highlight, sensitive bool, parser *lute.Lute) (out HistoryResult, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("history_render_panic:%T", v)
		}
		if err != nil {
			out = HistoryResult{}
		}
	}()
	if ctx == nil || parser == nil || parser.RenderOptions == nil || parser.ParseOptions == nil {
		return out, errors.New("history_render_unbound")
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	local := *parser
	options := *parser.RenderOptions
	local.RenderOptions = &options
	tree, e := dataparser.ParseJSONWithoutFix(raw, local.ParseOptions)
	if e != nil {
		return out, e
	}
	if tree == nil || tree.Root == nil {
		return out, errors.New("history_tree_invalid")
	}
	out.ID, out.RootID, out.Large = tree.Root.ID, tree.Root.ID, len(raw) >= 1<<20
	out.Title = tree.Root.IALAttr("title")
	if !out.Large {
		keywords := search.SplitKeyword(strings.Join(strings.Split(keyword, " "), search.TermSep))
		var unlinks []*ast.Node
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if !enter {
				return ast.WalkContinue
			}
			n.RemoveIALAttr("heading-fold")
			n.RemoveIALAttr("fold")
			if highlight && len(keywords) > 0 {
				MarkReplaceSpan(n, &unlinks, keywords, search.MarkDataType, &local, sensitive)
			}
			return ast.WalkContinue
		})
		if e = ctx.Err(); e != nil {
			return out, e
		}
		for _, n := range unlinks {
			n.Unlink()
		}
		renderTree := &parse.Tree{Root: &ast.Node{Type: ast.NodeDocument}}
		for n := tree.Root.FirstChild; n != nil; {
			next := n.Next
			renderTree.Root.AppendChild(n)
			n = next
		}
		tree = renderTree
	}
	local.RenderOptions.ProtyleContenteditable = false
	if out.Large {
		out.Content = string(render.NewFormatRenderer(tree, local.RenderOptions).Render())
	} else {
		out.Content = local.Tree2BlockDOM(tree, local.RenderOptions)
	}
	return out, ctx.Err()
}
