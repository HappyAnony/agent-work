// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeSQLQuery(ctx context.Context, statement string) (out []map[string]interface{}, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		settings := l.workspace.resources.configuration.Search
		if settings == nil {
			return indexdb.ErrProjection
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		out, e = indexdb.QueryMaps(ctx, db, statement, settings.Limit, false)
		return e
	})
	if err != nil {
		out = nil
	}
	return
}
