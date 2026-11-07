// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"errors"
)

// RepairQueries binds the original repair reads to one database. A failed scan
// returns an error, never an empty result which could authorize index deletion.
// Blocks intentionally bypasses value caches while auditing index consistency.
type RepairQueries struct{ DB *sql.DB }

var ErrRepairTable = errors.New("invalid_index_repair_table")

func (q RepairQueries) DuplicatedRoots(ctx context.Context, table string) (ids []string, err error) {
	switch table {
	case "blocks", "blocks_fts", "blocks_fts_case_insensitive":
	default:
		return nil, ErrRepairTable
	}
	rows, e := Query(ctx, q.DB, "SELECT DISTINCT root_id FROM `"+table+"` GROUP BY id HAVING COUNT(*) > 1")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	return ids, nil
}
func (q RepairQueries) RootUpdated(ctx context.Context) (map[string]string, error) {
	rows, e := Query(ctx, q.DB, "SELECT root_id, updated FROM blocks WHERE type = 'd'")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ret := map[string]string{}
	for rows.Next() {
		var id, updated string
		if e = rows.Scan(&id, &updated); e != nil {
			return nil, e
		}
		ret[id] = updated
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	return ret, nil
}
func (q RepairQueries) Blocks(ctx context.Context, ids []string) ([]*BlockRecord, error) {
	return (CachedBlocks{DB: q.DB}).GetMany(ctx, ids)
}
func (q RepairQueries) DuplicatedReferenceRoots(ctx context.Context) ([]string, error) {
	return (References{DB: q.DB}).DuplicatedDefinitionRoots(ctx)
}
