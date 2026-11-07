// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"github.com/88250/gulu"
	"regexp"
	"strconv"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/search"
)

// Original GetDoc highlight query: full highlight (not snippets), seven fields,
// root-local, at most 256 rows, and space-separated FTS terms joined with OR.
func DocumentKeywords(ctx context.Context, db *sql.DB, settings conf.Search, query string, types map[string]bool, method int, root string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filter := SearchTypeFilter(settings, types)
	q := &blockSearchRun{ctx: ctx, db: db, settings: settings}
	var fields []string
	if method == 3 {
		regex, err := regexp.Compile(query)
		if err != nil {
			return nil, err
		}
		stmt := "SELECT * FROM blocks WHERE " + q.fieldRegexp(query) + " AND type IN " + filter + " AND root_id='" + strings.ReplaceAll(root, "'", "''") + "'"
		for _, b := range q.selectRegex(stmt, regex, settings.Name, settings.Alias, settings.Memo, settings.IAL, 1, 256) {
			fields = append(fields, b.HPath, b.Name, b.Alias, b.Memo, b.Tag, b.IAL, b.Content)
		}
		if q.err != nil {
			return nil, q.err
		}
	} else if method == 0 || method == 1 {
		if method == 0 {
			query = strings.ReplaceAll(stringQuery(query), "''", "'")
		}
		query = strings.ReplaceAll(query, " ", " OR ")
		table := "blocks_fts"
		if !settings.CaseSensitive {
			table = "blocks_fts_case_insensitive"
		}
		var columns []string
		for _, column := range []int{6, 7, 8, 9, 10, 17, 11} {
			columns = append(columns, "highlight("+table+","+strconv.Itoa(column)+",'"+search.SearchMarkLeft+"','"+search.SearchMarkRight+"')")
		}
		rows, err := db.QueryContext(ctx, "SELECT "+strings.Join(columns, ",")+" FROM "+table+" WHERE "+table+" MATCH ? AND type IN "+filter+" AND root_id=? LIMIT 256", q.columnFilter()+":("+query+")", root)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var values [7]string
			if err = rows.Scan(&values[0], &values[1], &values[2], &values[3], &values[4], &values[5], &values[6]); err != nil {
				return nil, err
			}
			fields = append(fields, values[:]...)
		}
		if err = rows.Err(); err != nil {
			return nil, err
		}
	}
	var ret []string
	for _, field := range fields {
		ret = append(ret, gulu.Str.SubstringsBetween(field, search.SearchMarkLeft, search.SearchMarkRight)...)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return gulu.Str.RemoveDuplicatedElem(ret), nil
}
