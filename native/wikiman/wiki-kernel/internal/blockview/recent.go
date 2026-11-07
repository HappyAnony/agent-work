// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"context"
	"database/sql"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

// RecentUpdated is the inherited desktop/mobile query and original wire
// projection. Device policy and source ignore clauses are explicit inputs.
func RecentUpdated(ctx context.Context, db *sql.DB, settings conf.Search, mobile bool, ignore []string) ([]*Block, error) {
	statement := "SELECT * FROM blocks WHERE type = 'p' AND length > 1"
	if mobile {
		statement = "SELECT * FROM blocks WHERE type = 'd'"
	}
	for _, line := range ignore {
		statement += " AND " + line
	}
	statement += " ORDER BY updated DESC"
	rows, e := indexdb.SelectReadBlockRows(ctx, db, statement, 1, 16)
	if e != nil {
		return nil, e
	}
	out := make([]*Block, 0, len(rows))
	for i := range rows {
		out = append(out, FromRow(&rows[i], "", 0, settings.CaseSensitive))
	}
	return out, ctx.Err()
}
