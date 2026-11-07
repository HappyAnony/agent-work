// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"net/url"
	"strings"

	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/workspace"
)

func prepareUIClipboard(q struct{ DOM string }) (uiCall, error) {
	if len(q.DOM) > 512<<10 || strings.ContainsRune(q.DOM, '\x00') {
		return uiCall{}, ErrArgument
	}
	return uiCall{run: func(ctx context.Context, l *workspace.Lease) (any, error) {
		var result string
		err := l.WithConfiguredParser(ctx, nil, func(engine *lute.Lute) error {
			engine.SetSanitize(true)
			engine.SetHTMLTag2TextMark(true)
			engine.SetHTML2MarkdownAttrs([]string{"alias", "memo", "bookmark", "custom-*"})
			tree := engine.HTML2Tree(gulu.Str.RemovePUA(q.DOM))
			if tree == nil || tree.Root == nil {
				return ErrArgument
			}
			var rejected bool
			var withMath bool
			ast.Walk(tree.Root, func(node *ast.Node, entering bool) ast.WalkStatus {
				if !entering {
					return ast.WalkContinue
				}
				switch node.Type {
				case ast.NodeHTMLBlock:
					tokens := bytes.ToLower(node.Tokens)
					rejected = bytes.Contains(tokens, []byte("<svg")) || bytes.Contains(tokens, []byte("data:image/"))
				case ast.NodeInlineMath:
					withMath = true
				case ast.NodeLinkDest:
					dest := node.TokensStr()
					decoded, e := url.PathUnescape(dest)
					if e != nil {
						rejected = true
						break
					}
					decoded = strings.ToLower(decoded)
					rejected = strings.HasPrefix(decoded, "data:") || strings.HasPrefix(decoded, "file:") ||
						strings.Contains(decoded, "\\") || strings.HasPrefix(decoded, "/") && !strings.HasPrefix(decoded, "/assets/")
				}
				if rejected || ctx.Err() != nil {
					return ast.WalkStop
				}
				return ast.WalkContinue
			})
			if rejected {
				return ErrUIEndpoint
			}
			var err error
			result, err = docview.FormatHTMLPaste(engine, tree, withMath, ctx.Err, nil)
			return err
		})
		return result, err
	}}, nil
}
