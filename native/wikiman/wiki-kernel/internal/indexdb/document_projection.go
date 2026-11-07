// SPDX-License-Identifier: AGPL-3.0-or-later
// Reuse the original document branch of buildBlockFromNode without indexing
// children, resolving AV/assets/OCR or acquiring any database/source writer.
package indexdb

import (
	"context"
	"fmt"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
)

func ProjectDocumentBlock(ctx context.Context, tree *parse.Tree, parser *lute.Lute) (row *BlockRecord, err error) {
	defer func() {
		if value := recover(); value != nil {
			row = nil
			err = fmt.Errorf("%w: document projection panic (%T)", ErrProjection, value)
		}
	}()
	if ctx == nil || tree == nil || tree.Root == nil || tree.Root.Type != ast.NodeDocument || tree.ID != tree.Root.ID || parser == nil {
		return nil, ErrProjection
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	ial := make([][]string, len(tree.Root.KramdownIAL))
	for i, kv := range tree.Root.KramdownIAL {
		if len(kv) != 2 {
			return nil, ErrProjection
		}
		ial[i] = append([]string(nil), kv...)
	}
	// nodeHash sorts IAL, so only the detached root may be passed to it.
	root := &ast.Node{ID: tree.Root.ID, Type: ast.NodeDocument, Spec: tree.Root.Spec, KramdownIAL: ial}
	candidate := *tree
	candidate.Root = root
	p := &treeProjector{ctx: ctx, parser: parser}
	row, _ = p.buildBlockFromNode(root, &candidate)
	if p.err != nil {
		return nil, p.err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return row, nil
}
