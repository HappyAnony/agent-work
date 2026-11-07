// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"database/sql"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) VacuumNativeIndexes(ctx context.Context) (out []indexdb.VacuumSize, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		dbs := []*sql.DB{}
		for _, kind := range []indexdb.Kind{indexdb.Main, indexdb.History, indexdb.Assets} {
			db, e := b.Database(kind)
			if e != nil {
				return e
			}
			dbs = append(dbs, db)
		}
		var e error
		out, e = indexdb.Vacuum(ctx, dbs, func(c context.Context) error { return c.Err() })
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
