// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"bytes"
	"context"
	"database/sql"
	"github.com/88250/lute/ast"
	"strconv"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/search"
)

func SearchReferenceBlocks(ctx context.Context, db *sql.DB, settings conf.Search, keyword string, onlyDoc bool, ignoreLines []string) (ret []BlockRecord, err error) {
	if err = ctx.Err(); err != nil {
		return
	}
	keyword = FilterQueryInvisible(keyword)
	q := &blockSearchRun{ctx: ctx, db: db, settings: settings, strictRead: true}

	if id := ExtractReferenceID(keyword); "" != id {
		ret, _, _ = q.searchBySQL("SELECT * FROM `blocks` WHERE `id` = '"+id+"'", 36, 1, 32)
		return ret, q.err
	}

	quotedKeyword := stringQuery(keyword)
	table := "blocks_fts" // 大小写敏感
	if !settings.CaseSensitive {
		table = "blocks_fts_case_insensitive"
	}

	projections := "id, parent_id, root_id, hash, box, path, " +
		"snippet(" + table + ", 6, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS hpath, " +
		"snippet(" + table + ", 7, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS name, " +
		"snippet(" + table + ", 8, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS alias, " +
		"snippet(" + table + ", 9, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS memo, " +
		"snippet(" + table + ", 10, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS tag, " +
		"snippet(" + table + ", 11, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS content, " +
		"fcontent, markdown, length, type, subtype, ial, sort, created, updated"
	stmt := "SELECT " + projections + " FROM " + table + " WHERE " + table + " MATCH '" + q.columnFilter() + ":(" + quotedKeyword + ")' AND type"
	if onlyDoc {
		stmt += " = 'd'"
	} else {
		stmt += " IN " + settings.TypeFilter()
	}

	if 0 < len(ignoreLines) {
		// Support ignore search results https://github.com/siyuan-note/siyuan/issues/10089
		buf := bytes.Buffer{}
		for _, line := range ignoreLines {
			buf.WriteString(" AND ")
			buf.WriteString(line)
		}
		stmt += buf.String()
	}

	orderBy := ` ORDER BY CASE
             WHEN name = '${keyword}' THEN 10
             WHEN alias = '${keyword}' THEN 20
             WHEN memo = '${keyword}' THEN 30
             WHEN content = '${keyword}' and type = 'd' THEN 40
             WHEN content LIKE '%${keyword}%' and type = 'd' THEN 41
             WHEN name LIKE '%${keyword}%' THEN 50
             WHEN alias LIKE '%${keyword}%' THEN 60
             WHEN content = '${keyword}' and type = 'h' THEN 70
             WHEN content LIKE '%${keyword}%' and type = 'h' THEN 71
             WHEN fcontent = '${keyword}' and type = 'i' THEN 80
             WHEN fcontent LIKE '%${keyword}%' and type = 'i' THEN 81
             WHEN memo LIKE '%${keyword}%' THEN 90
             WHEN content LIKE '%${keyword}%' and type != 'i' and type != 'l' THEN 100
             ELSE 65535 END ASC, sort ASC, length ASC`
	orderBy = strings.ReplaceAll(orderBy, "${keyword}", strings.ReplaceAll(keyword, "'", "''"))
	stmt += orderBy + " LIMIT " + strconv.Itoa(settings.Limit)
	ret, err = SelectReadBlockRows(ctx, db, stmt, 1, settings.Limit)
	if err != nil {
		return nil, err
	}
	if 1 > len(ret) {
		ret = []BlockRecord{}
	}
	return ret, ctx.Err()
}
func ExtractReferenceID(content string) (ret string) {
	// Improve block ref search ID extraction https://github.com/siyuan-note/siyuan/issues/10848

	if 22 > len(content) {
		return
	}

	// 从第一个字符开始循环，直到找到一个合法的 ID 为止
	for i := 0; i < len(content)-21; i++ {
		if ast.IsNodeIDPattern(content[i : i+22]) {
			ret = content[i : i+22]
			return
		}
	}
	return
}
