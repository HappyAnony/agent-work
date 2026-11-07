// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
)

// ProjectBlock uses the same projection as indexing, without publishing rows.
func ProjectBlock(ctx context.Context, tree *parse.Tree, node *ast.Node, parser *lute.Lute, content ContentServices) (out *BlockRecord, err error) {
	defer func() {
		if v := recover(); v != nil {
			out = nil
			err = fmt.Errorf("%w: block projection panic (%T)", ErrProjection, v)
		}
	}()
	if ctx == nil || tree == nil || tree.Root == nil || node == nil || parser == nil {
		return nil, ErrProjection
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	p := &treeProjector{ctx: ctx, parser: parser, options: TreeServices{Content: content, PrepareOCR: func(c context.Context, _ *ast.Node) error { return c.Err() }}}
	out, _ = p.buildBlockFromNode(node, tree)
	if p.err != nil {
		return nil, p.err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return
}
