// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
)

func QuerySQLContext(ctx context.Context, statement string) (out []map[string]interface{}, err error) {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	return runOwnedSQLQuery(ctx, base, statement)
}
func runOwnedSQLQuery(ctx context.Context, base *legacyReindexOwner, statement string) (out []map[string]interface{}, err error) {
	err = base.run(ctx, func(c, _ context.Context) error {
		db := sql.CaptureLegacyRepairQueries().DB
		if db == nil {
			return indexdb.ErrNoDatabase
		}
		var e error
		limit := 0
		if e = base.owner.WithSettingSnapshot(c, func(v conf.AppData) error {
			if v.Search == nil {
				return conf.ErrBusinessUnbound
			}
			limit = v.Search.Limit
			return nil
		}); e != nil {
			return e
		}
		// The original administrator-only compatibility endpoint may edit derived
		// indexes. Read-only owners and native readers never receive this port.
		out, e = indexdb.QueryMaps(c, db, statement, limit, !base.owner.persistenceReadOnly)
		if e != nil {
			return e
		}
		if db != sql.CaptureLegacyRepairQueries().DB {
			return conf.ErrBusinessUnbound
		}
		return base.guard(c)
	})
	if err != nil {
		out = nil
	}
	return
}
