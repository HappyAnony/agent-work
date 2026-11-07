// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"text/template"
)

// TemplateFunctions preserves the old template function names and projections.
// SQL is owner-local, SELECT-only, cancellable and capped even with explicit LIMIT.
func TemplateFunctions(ctx context.Context, db *sql.DB) template.FuncMap {
	maps := func(stmt string, limit int) (ret []map[string]interface{}, err error) {
		if err = ValidateReadSQL(stmt); err != nil {
			return
		}
		rows, err := Query(ctx, db, stmt)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			if len(ret) >= limit {
				return nil, errors.New("template_query_limit")
			}
			v := make([]interface{}, len(cols))
			ptr := make([]interface{}, len(cols))
			for i := range v {
				ptr[i] = &v[i]
			}
			if err = rows.Scan(ptr...); err != nil {
				return nil, err
			}
			row := map[string]interface{}{}
			for i, c := range cols {
				row[c] = v[i]
			}
			ret = append(ret, row)
		}
		return ret, rows.Err()
	}
	replace := func(s string, args []string) string {
		for _, a := range args {
			s = strings.Replace(s, "?", a, 1)
		}
		return s
	}
	blocks := func(stmt string, args ...string) ([]BlockRecord, error) {
		stmt = replace(stmt, args)
		if e := ValidateReadSQL(stmt); e != nil {
			return nil, e
		}
		return SelectBlockRows(ctx, db, "SELECT * FROM ("+strings.TrimSuffix(strings.TrimSpace(stmt), ";")+") LIMIT 512", 1, 512)
	}
	return template.FuncMap{
		"queryBlocks": blocks,
		"getBlock": func(arg any) (*BlockRecord, error) {
			id := ""
			switch a := arg.(type) {
			case string:
				id = a
			case map[string]interface{}:
				id, _ = a["id"].(string)
			}
			if id == "" {
				return nil, nil
			}
			rows, e := blocks("SELECT * FROM blocks WHERE id = '" + strings.ReplaceAll(id, "'", "''") + "'")
			if e != nil || len(rows) == 0 {
				return nil, e
			}
			return &rows[0], nil
		},
		"querySQL": func(s string) ([]map[string]interface{}, error) { return maps(s, 1024) },
		"querySpans": func(s string, args ...string) (ret []*SpanRecord, err error) {
			s = replace(s, args)
			if err = ValidateReadSQL(s); err != nil {
				return
			}
			rows, err := Query(ctx, db, s)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			for rows.Next() {
				if len(ret) >= 512 {
					return nil, errors.New("template_query_limit")
				}
				v := &SpanRecord{}
				if err = rows.Scan(&v.ID, &v.BlockID, &v.RootID, &v.Box, &v.Path, &v.Content, &v.Markdown, &v.Type, &v.IAL); err != nil {
					return nil, err
				}
				ret = append(ret, v)
			}
			return ret, rows.Err()
		},
	}
}
