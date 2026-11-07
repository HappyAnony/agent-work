// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package indexdb

import (
	"context"
	"database/sql"
	"strings"
)

// References carries the original four reference aggregate queries. Its DB is
// supplied by the owner; no default/global database, parser or cache is used.
type References struct{ DB *sql.DB }

func (q References) DuplicatedDefinitionRoots(ctx context.Context) (ret []string, err error) {
	rows, err := Query(ctx, q.DB, "SELECT DISTINCT def_block_root_id FROM refs GROUP BY def_block_id, def_block_root_id, block_id HAVING COUNT(*) > 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return ret, err
		}
		ret = append(ret, id)
	}
	return ret, rows.Err()
}

func (q References) DefinitionCounts(ctx context.Context, ids []string) (map[string]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q.DB == nil {
		return nil, ErrNoDatabase
	}
	if len(ids) == 0 {
		return map[string]int{}, nil
	}
	// Bounded parameters preserve large original ID lists without exceeding
	// SQLite's variable limit. Repeated IDs keep IN semantics, not double counts.
	ret := map[string]int{}
	for start := 0; start < len(ids); start += 512 {
		end := min(start+512, len(ids))
		args := make([]interface{}, end-start)
		for i, id := range ids[start:end] {
			args[i] = id
		}
		counts, err := q.counts(ctx, "SELECT def_block_id, COUNT(*) FROM refs WHERE def_block_id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+") GROUP BY def_block_id", args...)
		if err != nil {
			return ret, err
		}
		for id, count := range counts {
			ret[id] = count
		}
	}
	return ret, nil
}

func (q References) RootChildrenCounts(ctx context.Context, root string) (map[string]int, error) {
	return q.counts(ctx, "SELECT def_block_id, COUNT(*) FROM refs WHERE def_block_root_id = ? GROUP BY def_block_id", root)
}

func (q References) RootCounts(ctx context.Context) (map[string]int, error) {
	return q.counts(ctx, "SELECT def_block_root_id, COUNT(DISTINCT block_id) FROM refs GROUP BY def_block_root_id")
}

func (q References) counts(ctx context.Context, statement string, args ...interface{}) (map[string]int, error) {
	ret := map[string]int{}
	rows, err := Query(ctx, q.DB, statement, args...)
	if err != nil {
		return ret, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return ret, err
		}
		ret[id] = count
	}
	return ret, rows.Err()
}
