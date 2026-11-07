// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"strings"
	"wiki-kernel/conf"
)

// SearchDocumentRoots preserves original hpath/NAM OR groups, keyword AND,
// box/depth order and SQL LIMIT semantics. Values are parameters, never SQL.
func SearchDocumentRoots(ctx context.Context, db *sql.DB, settings conf.Search, words, exclude []string) ([]BlockRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(words) == 0 {
		return []BlockRecord{}, nil
	}
	var groups []string
	var args []interface{}
	for _, word := range words {
		clauses := []string{"hpath LIKE ?"}
		args = append(args, "%"+word+"%")
		for _, field := range []struct {
			name    string
			enabled bool
		}{{"name", settings.Name}, {"alias", settings.Alias}, {"memo", settings.Memo}} {
			if field.enabled {
				clauses = append(clauses, field.name+" LIKE ?")
				args = append(args, "%"+strings.TrimSpace(word)+"%")
			}
		}
		groups = append(groups, "("+strings.Join(clauses, " OR ")+")")
	}
	stmt := "SELECT *, length(hpath) - length(replace(hpath, '/', '')) AS lv FROM blocks WHERE type='d' AND " + strings.Join(groups, " AND ")
	if len(exclude) > 0 {
		stmt += " AND root_id NOT IN (" + strings.TrimSuffix(strings.Repeat("?,", len(exclude)), ",") + ")"
		for _, id := range exclude {
			args = append(args, id)
		}
	}
	stmt += " ORDER BY box DESC,lv ASC LIMIT ?"
	args = append(args, settings.Limit)
	rows, err := Query(ctx, db, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ret := []BlockRecord{}
	for rows.Next() {
		var row BlockRecord
		var level int
		fields := append(blockFields(&row), &level)
		if err = rows.Scan(fields...); err != nil {
			return nil, err
		}
		ret = append(ret, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}
