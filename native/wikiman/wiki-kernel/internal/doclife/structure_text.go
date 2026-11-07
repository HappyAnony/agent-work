// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"context"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"unicode/utf8"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func HeadingTitle(ctx context.Context, n *ast.Node, max int, content indexdb.ContentServices) (string, error) {
	if n == nil || n.Type != ast.NodeHeading || max < 0 {
		return "", ErrDependency
	}
	text, e := docquery.BlockText(ctx, n, nil, true, func(c context.Context, n *ast.Node, exclude []string) (string, error) {
		return indexdb.StaticContent(c, n, exclude, false, false, false, content)
	})
	if e != nil {
		return "", e
	}
	if utf8.RuneCountInString(text) > max {
		text = gulu.Str.SubStr(text, max) + "..."
	}
	return text, ctx.Err()
}
