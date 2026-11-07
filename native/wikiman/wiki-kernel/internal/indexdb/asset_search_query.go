// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/88250/vitess-sqlparser/sqlparser"
	"strconv"
	"strings"
)

func normalizeAssetSQL(stmt string) string {
	stmt = strings.ReplaceAll(stmt, "\\'", "''")
	stmt = strings.ReplaceAll(stmt, "\\\"", "\"")
	stmt = strings.ReplaceAll(stmt, "\\\\*", "\\*")
	return strings.ReplaceAll(stmt, "from dual", "")
}

// SelectAssetContentsPage retains the original parsed-query pagination rule;
// parse fallback is still a validated SELECT and receives explicit pagination.
func SelectAssetContentsPage(ctx context.Context, db *sql.DB, stmt string, page, limit int) ([]AssetContentEntry, error) {
	if ctx == nil || db == nil {
		return nil, ErrNoDatabase
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if page < 1 || limit < 1 || limit > 10000 {
		return nil, fmt.Errorf("invalid_asset_search_page")
	}
	if e := ValidateReadSQL(stmt); e != nil {
		return nil, e
	}
	parsed, e := sqlparser.Parse(stmt)
	if e == nil {
		if s, ok := parsed.(*sqlparser.Select); ok {
			if s.Limit != nil && s.Limit.Rowcount != nil {
				if n, ok := s.Limit.Rowcount.(*sqlparser.SQLVal); ok {
					if count, err := strconv.Atoi(string(n.Val)); err == nil && count > 0 {
						limit = count
					}
				}
			}
			if limit > 10000 {
				return nil, fmt.Errorf("invalid_asset_search_page")
			}
			s.Limit = &sqlparser.Limit{Rowcount: &sqlparser.SQLVal{Type: sqlparser.IntVal, Val: []byte(strconv.Itoa(limit))}, Offset: &sqlparser.SQLVal{Type: sqlparser.IntVal, Val: []byte(strconv.Itoa((page - 1) * limit))}}
			stmt = normalizeAssetSQL(sqlparser.String(s))
		} else {
			return nil, fmt.Errorf("asset_search_requires_select")
		}
	} else {
		stmt = "SELECT * FROM (" + strings.TrimSuffix(strings.TrimSpace(stmt), ";") + ") AS asset_page LIMIT " + strconv.Itoa(limit) + " OFFSET " + strconv.Itoa((page-1)*limit)
	}
	if e := ValidateReadSQL(stmt); e != nil {
		return nil, e
	}
	out, e := SelectAssetContents(ctx, db, stmt, limit)
	if e != nil {
		return nil, e
	}
	return out, ctx.Err()
}

func CountAssetContents(ctx context.Context, db *sql.DB, stmt string) (int, error) {
	if ctx == nil || db == nil {
		return 0, ErrNoDatabase
	}
	if e := ValidateReadSQL(stmt); e != nil {
		return 0, e
	}
	rows, e := Query(ctx, db, stmt)
	if e != nil {
		return 0, e
	}
	defer rows.Close()
	if !rows.Next() {
		if e = rows.Err(); e != nil {
			return 0, e
		}
		return 0, fmt.Errorf("asset_count_missing")
	}
	var count int
	if e = rows.Scan(&count); e != nil {
		return 0, e
	}
	if e = rows.Err(); e != nil {
		return 0, e
	}
	return count, ctx.Err()
}

func CountAssetQuery(ctx context.Context, db *sql.DB, stmt string) (int, error) {
	if e := ValidateReadSQL(stmt); e != nil {
		return 0, e
	}
	base := normalizeAssetSQL(removeLimitClause(stmt))
	base = strings.TrimSuffix(strings.TrimSpace(base), ";")
	return CountAssetContents(ctx, db, "SELECT COUNT(*) FROM ("+base+") AS matched_assets")
}

func QuoteSearchKeyword(query string) string { return stringQuery(query) }
