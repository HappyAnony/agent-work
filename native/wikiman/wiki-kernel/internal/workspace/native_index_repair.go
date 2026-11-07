// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

// InspectNativeIndexRepair reads the currently materialized indexes under one
// owner admission/gate. It never flushes pending work, scans/repairs source,
// mutates indexes or grants source write authority. A plan is diagnostic only.
func (l *Lease) InspectNativeIndexRepair(ctx context.Context) (out indexrepair.Plan, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r := l.workspace.resources
		if r.configuration.Search == nil {
			return conf.ErrBusinessUnbound
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		tree, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		out, e = indexrepair.Inspect(ctx, indexdb.RepairQueries{DB: db}, indexdb.BlockTrees{DB: tree}, r.configuration.Search.CaseSensitive)
		return e
	})
	if err != nil {
		return indexrepair.Plan{}, err
	}
	return out, nil
}
