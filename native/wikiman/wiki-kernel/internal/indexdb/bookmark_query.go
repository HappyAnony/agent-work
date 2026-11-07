// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

// BookmarkBlocks preserves the original broad candidate query. Mutation must
// still compare the current AST's bookmark exactly, rather than trusting SQL.
func BookmarkBlocks(ctx context.Context, db *sql.DB) (ret []BlockRecord, err error) {
	rows, err := Query(ctx, db, "SELECT * FROM blocks WHERE ial LIKE ?", "%bookmark=%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		b, e := scanBlock(rows)
		if e != nil {
			return nil, e
		}
		ret = append(ret, b)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}

// BookmarkLabels retains the original raw IAL substring extraction, including
// custom-bookmark candidates. The panel separately groups parsed bookmark keys.
// Malformed boundaries fail explicitly instead of slicing past a corrupt row.
func BookmarkLabels(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := BookmarkBlocks(ctx, db)
	if err != nil {
		return nil, err
	}
	labels := map[string]bool{}
	for _, row := range rows {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		i := strings.Index(row.IAL, "bookmark")
		if i < 0 {
			continue
		}
		start := i + len("bookmark") + 2
		if start > len(row.IAL) {
			return nil, errors.New("invalid_bookmark_ial")
		}
		rest := row.IAL[start:]
		end := strings.Index(rest, "\"")
		if end < 0 {
			return nil, errors.New("invalid_bookmark_ial")
		}
		if value := rest[:end]; value != "" {
			labels[value] = true
		}
	}
	ret := []string{}
	for label := range labels {
		ret = append(ret, label)
	}
	sort.Strings(ret)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}
