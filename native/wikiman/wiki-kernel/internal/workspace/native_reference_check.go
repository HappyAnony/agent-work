// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"slices"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeCheckBlockRef(ctx context.Context, ids []string) (out bool, err error) {
	ids = slices.Clone(ids)
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if !r.blockTreeReady {
			if e := r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		bt, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		out, e = docquery.CheckReferences(ctx, ids, func(c context.Context, ids []string) ([]indexdb.BlockTreeRecord, error) {
			return (indexdb.BlockTrees{DB: bt}).Query(c, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
		}, indexdb.References{DB: db})
		return e
	})
	if err != nil {
		return false, err
	}
	return
}
