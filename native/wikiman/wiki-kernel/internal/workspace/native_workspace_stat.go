// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/workspacestat"
)

func (l *Lease) NativeWorkspaceStatistics(ctx context.Context) (out workspacestat.Result, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if !r.blockTreeReady {
			if e := r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		db, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		out, e = workspacestat.Measure(ctx, indexrepair.Source{Data: w.data}, func(c context.Context, trees bool) (int, error) {
			filter := indexdb.BlockTreeFilter{Kind: "all"}
			if trees {
				filter = indexdb.BlockTreeFilter{Kind: "type", Value: "d"}
			}
			return (indexdb.BlockTrees{DB: db}).Count(c, filter)
		})
		return e
	})
	if err != nil {
		return workspacestat.Result{}, err
	}
	return
}
