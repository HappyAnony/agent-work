// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docview"
)

func loadNodesByStartEndWithContext(ctx context.Context, tree *parse.Tree, start, end string, limit int) ([]*ast.Node, bool, error) {
	return docview.LoadNodesByStartEnd(ctx, tree, start, end, limit)
}
func loadNodesByModeWithContext(ctx context.Context, n *ast.Node, index, mode, size int, doc, heading bool) ([]*ast.Node, bool, error) {
	return docview.LoadNodesByMode(ctx, n, index, mode, size, doc, heading)
}
