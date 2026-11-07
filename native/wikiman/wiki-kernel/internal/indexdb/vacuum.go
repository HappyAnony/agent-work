// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
)

type VacuumSize struct{ Before, After int64 }

func vacuumFile(ctx context.Context, db *sql.DB) (name string, err error) {
	rows, e := db.QueryContext(ctx, "PRAGMA database_list")
	if e != nil {
		return "", e
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var seq int
		var schema, filename string
		if e = rows.Scan(&seq, &schema, &filename); e != nil {
			return "", e
		}
		if schema == "main" {
			name = filename
		}
	}
	return name, rows.Err()
}
func vacuumSize(name string) (int64, error) {
	if name == "" {
		return 0, nil
	}
	info, e := os.Stat(name)
	if os.IsNotExist(e) {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("index_vacuum_path")
	}
	return info.Size(), nil
}

// Vacuum operates on already selected disposable indexes only. The caller
// keeps their owner alive and supplies a guard; no database is rediscovered.
func Vacuum(ctx context.Context, databases []*sql.DB, guard func(context.Context) error) (out []VacuumSize, err error) {
	if ctx == nil || guard == nil {
		return nil, errors.New("index_vacuum_unbound")
	}
	for _, db := range databases {
		if e := guard(ctx); e != nil {
			return nil, e
		}
		if db == nil {
			continue
		}
		name, e := vacuumFile(ctx, db)
		if e != nil {
			return nil, e
		}
		before, e := vacuumSize(name)
		if e != nil {
			return nil, e
		}
		if _, e = db.ExecContext(ctx, "VACUUM"); e != nil {
			return nil, e
		}
		after, e := vacuumSize(name)
		if e != nil {
			return nil, e
		}
		out = append(out, VacuumSize{Before: before, After: after})
	}
	if e := guard(ctx); e != nil {
		return nil, e
	}
	return out, nil
}
