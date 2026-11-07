// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// CloneReadProjection gives editing an isolated SQL view of its own candidates.
// The caller holds the source bundle's owner gate. No candidate is published.
func (b *Bundle) CloneReadProjection(ctx context.Context) (out *Bundle, err error) {
	out, err = OpenMemory(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, out.Close())
			out = nil
		}
	}()
	tables := map[Kind][]string{Main: {"stat", "blocks", "blocks_fts", "blocks_fts_case_insensitive", "spans", "assets", "attributes", "refs", "file_annotation_refs"}, BlockTree: {"blocktrees"}}
	for kind, names := range tables {
		source, e := b.Database(kind)
		if e != nil {
			return out, e
		}
		err = out.Transaction(ctx, kind, func(tx *sql.Tx) error {
			for _, name := range names {
				if _, e := tx.ExecContext(ctx, "DELETE FROM "+name); e != nil {
					return e
				}
				rows, e := source.QueryContext(ctx, "SELECT * FROM "+name)
				if e != nil {
					return e
				}
				e = func() (err error) {
					defer func() { err = errors.Join(err, rows.Close()) }()
					columns, err := rows.Columns()
					if err != nil {
						return err
					}
					query := "INSERT INTO " + name + " VALUES (" + strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",") + ")"
					for rows.Next() {
						values := make([]any, len(columns))
						refs := make([]any, len(columns))
						for i := range values {
							refs[i] = &values[i]
						}
						if err = rows.Scan(refs...); err != nil {
							return err
						}
						if _, err = tx.ExecContext(ctx, query, values...); err != nil {
							return err
						}
					}
					return rows.Err()
				}()
				if e != nil {
					return e
				}
			}
			return ctx.Err()
		})
		if err != nil {
			return out, err
		}
	}
	return out, ctx.Err()
}
