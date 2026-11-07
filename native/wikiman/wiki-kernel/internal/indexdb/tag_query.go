// SPDX-License-Identifier: AGPL-3.0-or-later
// Original sql/span.go and model.SearchTags algorithms with explicit DB/config.
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"wiki-kernel/internal/markup"
	"wiki-kernel/search"
)

func TagSpans(ctx context.Context, db *sql.DB, kind, value string, limit int) (ret []SpanRecord, err error) {
	stmt := "SELECT * FROM spans WHERE type LIKE '%tag%'"
	var args []interface{}
	switch kind {
	case "label":
		stmt += " AND content LIKE ? GROUP BY block_id"
		args = append(args, "%"+value+"%")
	case "keyword":
		for _, word := range strings.Split(value, " ") {
			stmt += " AND content LIKE ?"
			args = append(args, "%"+word+"%")
		}
		stmt += " GROUP BY markdown LIMIT ?"
		args = append(args, limit)
	case "path":
		if value != "" {
			stmt += " AND path=?"
			args = append(args, value)
		}
	default:
		return nil, errors.New("invalid_tag_query_kind")
	}
	rows, err := Query(ctx, db, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s SpanRecord
		if err = rows.Scan(&s.ID, &s.BlockID, &s.RootID, &s.Box, &s.Path, &s.Content, &s.Markdown, &s.Type, &s.IAL); err != nil {
			return nil, err
		}
		ret = append(ret, s)
	}
	return ret, rows.Err()
}
func SearchTagLabels(ctx context.Context, db *sql.DB, keyword string, limit int, caseSensitive bool) (ret []string, err error) {
	spans, err := TagSpans(ctx, db, "keyword", keyword, limit)
	if err != nil {
		return nil, err
	}
	labels := map[string]bool{}
	for _, s := range spans {
		labels[s.Content] = true
	}
	keyword = strings.Join(strings.Split(keyword, " "), search.TermSep)
	ret = []string{}
	for label := range labels {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if keyword == "" {
			ret = append(ret, markup.EscapeHTML(label))
		} else {
			_, text := search.MarkText(label, keyword, 1024, caseSensitive)
			ret = append(ret, text)
		}
	}
	sort.Strings(ret)
	return ret, nil
}
