// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"strconv"
	"strings"
	"time"
)

type HistorySearchRequest struct {
	Query, Box, Operation     string
	Type, Page, RetentionDays int
	Now                       time.Time
}
type HistorySearchResult struct {
	Created      []string
	Pages, Total int
}

const HistorySearchPageSize = 32

func historySearchFilter(q HistorySearchRequest) (string, []any, error) {
	if q.Type < 0 || q.Type > 3 || q.RetentionDays < 0 || q.Now.IsZero() {
		return "", nil, errors.New("history_search_request_invalid")
	}
	query := gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(q.Query))
	where := []string{"CAST(created AS INTEGER) > ?"}
	args := []any{q.Now.Add(-24 * time.Hour * time.Duration(q.RetentionDays)).Unix()}
	if query != "" {
		if q.Type == 3 {
			where = append(where, "id = ?")
			args = append(args, query)
		} else {
			// stringQuery's apostrophe escaping belongs to SQL literals. This MATCH
			// value is bound separately and must retain the original apostrophes.
			query = strings.ReplaceAll(stringQuery(query), "''", "'")
			fields := "title content"
			if q.Type == 0 {
				fields = "title"
			}
			where = append(where, "histories_fts_case_insensitive MATCH ?")
			args = append(args, "{"+fields+"}:("+query+")")
		}
	}
	if q.Operation != "all" {
		where = append(where, "op = ?")
		args = append(args, q.Operation)
	}
	box := q.Box
	if box != "%" && !ast.IsNodeIDPattern(box) {
		box = "%"
	}
	if q.Type == 0 || q.Type == 1 {
		where = append(where, "path LIKE ?", "path LIKE ?")
		args = append(args, "%/"+box+"/%", "%.docj")
	}
	if q.Type == 2 {
		where = append(where, "path LIKE ?")
		args = append(args, "%/assets/%")
	}
	return strings.Join(where, " AND "), args, nil
}

func SearchHistory(ctx context.Context, db *sql.DB, q HistorySearchRequest) (out HistorySearchResult, err error) {
	if ctx == nil || db == nil {
		return out, ErrNoDatabase
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if q.Page < 1 {
		return out, errors.New("history_search_page_invalid")
	}
	where, args, e := historySearchFilter(q)
	if e != nil {
		return out, e
	}
	if e = db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT created) FROM histories_fts_case_insensitive WHERE "+where, args...).Scan(&out.Total); e != nil {
		return HistorySearchResult{}, e
	}
	out.Created = []string{}
	out.Pages = (out.Total + HistorySearchPageSize - 1) / HistorySearchPageSize
	if q.Page > out.Pages {
		return out, ctx.Err()
	}
	values := append(append([]any(nil), args...), HistorySearchPageSize, (q.Page-1)*HistorySearchPageSize)
	rows, e := db.QueryContext(ctx, "SELECT DISTINCT created FROM histories_fts_case_insensitive WHERE "+where+" ORDER BY created DESC LIMIT ? OFFSET ?", values...)
	if e != nil {
		return HistorySearchResult{}, e
	}
	defer func() {
		err = errors.Join(err, rows.Close())
		if err != nil {
			out = HistorySearchResult{}
		}
	}()
	for rows.Next() {
		var created string
		if e = rows.Scan(&created); e != nil {
			return HistorySearchResult{}, e
		}
		out.Created = append(out.Created, created)
	}
	if e = rows.Err(); e != nil {
		return HistorySearchResult{}, e
	}
	return out, ctx.Err()
}

func SearchHistoryItems(ctx context.Context, db *sql.DB, q HistorySearchRequest, created string) (out []HistoryEntry, err error) {
	if ctx == nil || db == nil {
		return nil, ErrNoDatabase
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if _, e := strconv.Atoi(created); e != nil {
		return []HistoryEntry{}, nil
	}
	where, args, e := historySearchFilter(q)
	if e != nil {
		return nil, e
	}
	args = append(args, created, HistorySearchPageSize)
	rows, e := db.QueryContext(ctx, "SELECT id,type,op,title,content,path,created FROM histories_fts_case_insensitive WHERE "+where+" AND created = ? ORDER BY created DESC LIMIT ?", args...)
	if e != nil {
		return nil, e
	}
	defer func() {
		err = errors.Join(err, rows.Close())
		if err != nil {
			out = nil
		}
	}()
	out = []HistoryEntry{}
	for rows.Next() {
		var v HistoryEntry
		if e = rows.Scan(&v.ID, &v.Type, &v.Op, &v.Title, &v.Content, &v.Path, &v.Created); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	return out, ctx.Err()
}
