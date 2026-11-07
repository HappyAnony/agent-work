// SPDX-License-Identifier: AGPL-3.0-or-later
package assetquery

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/88250/go-humanize"
	"github.com/88250/lute/ast"
	"maps"
	"regexp"
	"slices"
	"strings"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/markup"
	"wiki-kernel/search"
)

type Content struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Ext     string `json:"ext"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	HSize   string `json:"hSize"`
	Updated int64  `json:"updated"`
	Content string `json:"content"`
}
type Request struct {
	Query                           string
	Types                           map[string]bool
	Method, OrderBy, Page, PageSize int
}
type Result struct {
	Contents           []*Content
	Matched, PageCount int
}

const table = "asset_contents_fts_case_insensitive"

var stableContentID = regexp.MustCompile(`^[0-9a-f]{64}$`)

func literal(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func typeClause(types map[string]bool) string {
	if types == nil {
		return ""
	}
	keys := []string{}
	for k, on := range types {
		if on {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	if len(keys) == 0 {
		return " AND 0"
	}
	for i, k := range keys {
		keys[i] = literal(k)
	}
	return " AND ext IN (" + strings.Join(keys, ",") + ")"
}
func orderClause(order int) string {
	switch order {
	case 1:
		return " ORDER BY rank ASC"
	case 2:
		return " ORDER BY updated ASC"
	case 3:
		return " ORDER BY updated DESC"
	default:
		return " ORDER BY rank DESC"
	}
}
func project(a indexdb.AssetContentEntry) *Content {
	text := markup.EscapeHTML(a.Content)
	text = strings.ReplaceAll(text, search.SearchMarkLeft, "<mark>")
	text = strings.ReplaceAll(text, search.SearchMarkRight, "</mark>")
	return &Content{ID: a.ID, Name: a.Name, Ext: a.Ext, Path: a.Path, Size: a.Size, HSize: humanize.BytesCustomCeil(uint64(a.Size), 2), Updated: a.Updated, Content: text}
}

// Search shares the original four asset search modes and presentation, while
// every SQL operation uses the supplied owner DB/context and returns errors.
func Search(ctx context.Context, db *sql.DB, q Request) (out Result, err error) {
	if ctx == nil || db == nil {
		return out, indexdb.ErrNoDatabase
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 10000 {
		return out, fmt.Errorf("invalid_asset_search_page")
	}
	q.Types = maps.Clone(q.Types)
	q.Query = indexdb.FilterQueryInvisible(strings.TrimSpace(q.Query))
	out.Contents = []*Content{}
	var stmt, countStmt string
	if q.Method == 2 {
		stmt = q.Query
	} else {
		var filter string
		if q.Method == 3 {
			if _, err = regexp.Compile(q.Query); err != nil {
				return Result{}, err
			}
			filter = "(name REGEXP " + literal(q.Query) + " OR content REGEXP " + literal(q.Query) + ")"
		} else {
			query := q.Query
			if q.Method != 1 {
				query = strings.ReplaceAll(indexdb.QuoteSearchKeyword(query), "''", "'")
			}
			filter = "`" + table + "` MATCH " + literal("{name content}:("+query+")")
		}
		columns := "id,name,ext,path,size,updated,content"
		if q.Method != 3 {
			columns = "id,name,ext,path,size,updated,snippet(" + table + ",6,'" + search.SearchMarkLeft + "','" + search.SearchMarkRight + "','...',64) AS content"
		}
		where := " FROM " + table + " WHERE " + filter + typeClause(q.Types)
		stmt = "SELECT " + columns + where + orderClause(q.OrderBy)
		countStmt = "SELECT COUNT(path)" + where
	}
	rows, err := indexdb.SelectAssetContentsPage(ctx, db, stmt, q.Page, q.PageSize)
	if err != nil {
		return Result{}, err
	}
	if q.Method == 2 {
		out.Matched, err = indexdb.CountAssetQuery(ctx, db, stmt)
	} else {
		out.Matched, err = indexdb.CountAssetContents(ctx, db, countStmt)
	}
	if err != nil {
		return Result{}, err
	}
	for _, a := range rows {
		if err = ctx.Err(); err != nil {
			return Result{}, err
		}
		out.Contents = append(out.Contents, project(a))
	}
	out.PageCount = (out.Matched + q.PageSize - 1) / q.PageSize
	return out, ctx.Err()
}

func Detail(ctx context.Context, db *sql.DB, id, query string, method int) (*Content, error) {
	if ctx == nil || db == nil {
		return nil, indexdb.ErrNoDatabase
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if !ast.IsNodeIDPattern(id) && !stableContentID.MatchString(id) {
		return nil, nil
	}
	filter := "id=" + literal(id)
	columns := "id,name,ext,path,size,updated,content"
	if query != "" && (method == 0 || method == 1) {
		if method == 0 {
			query = strings.ReplaceAll(indexdb.QuoteSearchKeyword(query), "''", "'")
		}
		filter += " AND `" + table + "` MATCH " + literal("{name content}:("+query+")")
		columns = "id,name,ext,path,size,updated,highlight(" + table + ",6,'" + search.SearchMarkLeft + "','" + search.SearchMarkRight + "') AS content"
	}
	rows, e := indexdb.SelectAssetContentsPage(ctx, db, "SELECT "+columns+" FROM "+table+" WHERE "+filter, 1, 1)
	if e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return nil, nil
	}
	a := rows[0]
	if query != "" && method == 3 {
		exp, e := regexp.Compile(query)
		if e != nil {
			return nil, e
		}
		a.Content = exp.ReplaceAllStringFunc(a.Content, func(hit string) string { return search.SearchMarkLeft + hit + search.SearchMarkRight })
	}
	out := project(a)
	out.Content = strings.ReplaceAll(out.Content, "\n", "<br>")
	return out, ctx.Err()
}
