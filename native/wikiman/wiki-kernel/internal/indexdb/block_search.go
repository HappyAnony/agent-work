// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model/search.go SQL search business, with explicit DB/config/errors.
package indexdb

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/vitess-sqlparser/sqlparser"
	sqlparser2 "github.com/rqlite/sql"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/search"
)

type BlockSearchRequest struct {
	Query                           string
	Boxes, Paths                    []string
	Types                           map[string]bool
	Method, OrderBy, Page, PageSize int
	Ignore                          []string
}
type BlockSearchResult struct {
	Blocks                                 []BlockRecord
	MatchedBlocks, MatchedRoots, PageCount int
	DocumentMode                           bool
	Terms                                  string
}
type blockSearchRun struct {
	ctx        context.Context
	db         *sql.DB
	settings   conf.Search
	err        error
	terms      string
	strictRead bool
}

func ValidateReadSQL(stmt string) error {
	p := sqlparser2.NewParser(strings.NewReader(normalizeSearchIdentifiers(stmt)))
	s, e := p.ParseStatement()
	if e != nil {
		return e
	}
	if _, ok := s.(*sqlparser2.SelectStatement); !ok {
		return fmt.Errorf("search_requires_select")
	}
	if _, e = p.ParseStatement(); e != io.EOF {
		return fmt.Errorf("search_requires_single_statement")
	}
	return nil
}

// The inherited queries use SQLite backtick identifiers, which the validation
// parser does not accept. Normalize only identifiers, never quoted values or
// comments, and still execute the original statement in SQLite.
func normalizeSearchIdentifiers(stmt string) string {
	var out strings.Builder
	for i := 0; i < len(stmt); {
		start := i
		if strings.HasPrefix(stmt[i:], "--") {
			for i < len(stmt) && stmt[i] != '\n' {
				i++
			}
		} else if strings.HasPrefix(stmt[i:], "/*") {
			i += 2
			for i < len(stmt) && !strings.HasPrefix(stmt[i:], "*/") {
				i++
			}
			if i < len(stmt) {
				i += 2
			}
		} else if stmt[i] == '\'' || stmt[i] == '"' {
			quote := stmt[i]
			i++
			for i < len(stmt) {
				if stmt[i] != quote {
					i++
					continue
				}
				i++
				if i < len(stmt) && stmt[i] == quote {
					i++
					continue
				}
				break
			}
		} else if stmt[i] == '`' {
			i++
			out.WriteByte('"')
			for i < len(stmt) {
				ch := stmt[i]
				i++
				if ch == '`' {
					if i < len(stmt) && stmt[i] == '`' {
						i++
						out.WriteByte('`')
						continue
					}
					out.WriteByte('"')
					break
				}
				if ch == '"' {
					out.WriteByte('"')
				}
				out.WriteByte(ch)
			}
			continue
		} else {
			i++
		}
		out.WriteString(stmt[start:i])
	}
	return out.String()
}

// SearchAllBlocks is an explicit internal replacement query, not an unbounded
// page size accepted by ordinary search endpoints. SQL remains read-only.
func SearchAllBlocks(ctx context.Context, db *sql.DB, settings conf.Search, request BlockSearchRequest) (BlockSearchResult, error) {
	request.Page, request.PageSize = 1, math.MaxInt
	ret, err := searchBlocks(ctx, db, settings, request, true)
	if err != nil {
		return BlockSearchResult{}, err
	}
	return ret, nil
}
func SearchBlocks(ctx context.Context, db *sql.DB, settings conf.Search, request BlockSearchRequest) (BlockSearchResult, error) {
	return searchBlocks(ctx, db, settings, request, false)
}
func searchBlocks(ctx context.Context, db *sql.DB, settings conf.Search, request BlockSearchRequest, all bool) (ret BlockSearchResult, err error) {
	defer func() {
		if value := recover(); value != nil {
			ret = BlockSearchResult{}
			err = fmt.Errorf("%w: search projection (%T)", ErrProjection, value)
		}
	}()
	if request.Page <= 0 || request.PageSize <= 0 || (!all && request.PageSize > 10000) {
		return ret, fmt.Errorf("invalid_search_page")
	}
	if err = ctx.Err(); err != nil {
		return ret, err
	}
	ret.Blocks = []BlockRecord{}
	if request.Query == "" {
		return ret, nil
	}
	q := &blockSearchRun{ctx: ctx, db: db, settings: settings, strictRead: true}
	query := FilterQueryInvisible(request.Query)
	boxFilter, pathFilter := buildBoxesFilter(request.Boxes), buildPathsFilter(request.Paths)
	typeFilter := SearchTypeFilter(settings, request.Types)
	orderBy := buildOrderBy(query, request.Method, request.OrderBy)
	ignore := ""
	for _, line := range request.Ignore {
		ignore += " AND " + line
	}
	page, size := request.Page, request.PageSize
	switch request.Method {
	case 2:
		if err = ValidateReadSQL(query); err != nil {
			return ret, err
		}
		ret.Blocks, ret.MatchedBlocks, ret.MatchedRoots = q.searchBySQL(query, 36, page, size)
	case 3:
		ret.Blocks, ret.MatchedBlocks, ret.MatchedRoots = q.fullTextSearchByRegexp(query, boxFilter, pathFilter, typeFilter, ignore, orderBy, 36, page, size)
	default:
		if ast.IsNodeIDPattern(query) {
			ret.Blocks, ret.MatchedBlocks, ret.MatchedRoots = q.searchBySQL("SELECT * FROM blocks WHERE id='"+query+"'", 36, page, size)
		} else if request.Method == 1 || len(strings.Split(strings.TrimSpace(query), " ")) < 2 {
			if request.Method != 1 {
				query = stringQuery(query)
			}
			ret.Blocks, ret.MatchedBlocks, ret.MatchedRoots = q.fullTextSearchByFTS(query, boxFilter, pathFilter, typeFilter, ignore, orderBy, 36, page, size)
		} else {
			ret.DocumentMode = true
			ret.Blocks, ret.MatchedBlocks, ret.MatchedRoots = q.fullTextSearchByLikeWithRoot(query, boxFilter, pathFilter, typeFilter, ignore, orderBy, 36, page, size)
		}
	}
	if q.err != nil {
		return BlockSearchResult{}, q.err
	}
	ret.Terms = q.terms
	ret.PageCount = ret.MatchedBlocks / size
	if ret.MatchedBlocks%size != 0 {
		ret.PageCount++
	}
	return ret, ctx.Err()
}
func FilterQueryInvisible(query string) string {
	query = strings.ReplaceAll(query, "　", "_@full_width_space@_")
	query = strings.ReplaceAll(query, "\t", "_@tab@_")
	query = strings.ReplaceAll(query, string(gulu.ZWJ), "__@ZWJ@__")
	query = gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(query))
	query = strings.ReplaceAll(query, "_@full_width_space@_", "　")
	query = strings.ReplaceAll(query, "_@tab@_", "\t")
	query = strings.ReplaceAll(query, "__@ZWJ@__", string(gulu.ZWJ))
	return strings.ReplaceAll(query, string(gulu.ZWJ)+"#", "#")
}
func SearchTypeFilter(settings conf.Search, types map[string]bool) string {
	s := settings
	if types != nil {
		s.Document = types["document"]
		s.Heading = types["heading"]
		s.List = types["list"]
		s.ListItem = types["listItem"]
		s.CodeBlock = types["codeBlock"]
		s.MathBlock = types["mathBlock"]
		s.Table = types["table"]
		s.Blockquote = types["blockquote"]
		s.SuperBlock = types["superBlock"]
		s.Paragraph = types["paragraph"]
		s.HTMLBlock = types["htmlBlock"]
		s.EmbedBlock = types["embedBlock"]
		s.DatabaseBlock = types["databaseBlock"]
		s.AudioBlock = types["audioBlock"]
		s.VideoBlock = types["videoBlock"]
		s.IFrameBlock = types["iframeBlock"]
		s.WidgetBlock = types["widgetBlock"]
	}
	return s.TypeFilter()
}
func buildBoxesFilter(boxes []string) string {
	if 0 == len(boxes) {
		return ""
	}
	builder := bytes.Buffer{}
	builder.WriteString(" AND (")
	for i, box := range boxes {
		builder.WriteString(fmt.Sprintf("box = '%s'", strings.ReplaceAll(box, "'", "''")))
		if i < len(boxes)-1 {
			builder.WriteString(" OR ")
		}
	}
	builder.WriteString(")")
	return builder.String()
}
func buildPathsFilter(paths []string) string {
	if 0 == len(paths) {
		return ""
	}
	builder := bytes.Buffer{}
	builder.WriteString(" AND (")
	for i, path := range paths {
		builder.WriteString(fmt.Sprintf("path LIKE '%s%%'", strings.ReplaceAll(path, "'", "''")))
		if i < len(paths)-1 {
			builder.WriteString(" OR ")
		}
	}
	builder.WriteString(")")
	return builder.String()
}
func buildOrderBy(query string, method, orderBy int) string {
	switch orderBy {
	case 1:
		return "ORDER BY created ASC"
	case 2:
		return "ORDER BY created DESC"
	case 3:
		return "ORDER BY updated ASC"
	case 4:
		return "ORDER BY updated DESC"
	case 6:
		if 0 != method && 1 != method {
			// 只有关键字搜索和查询语法搜索才支持按相关度升序 https://github.com/siyuan-note/siyuan/issues/7861
			return "ORDER BY sort DESC, updated DESC"
		}
		return "ORDER BY rank DESC" // 默认是按相关度降序，所以按相关度升序要反过来使用 DESC
	case 7:
		if 0 != method && 1 != method {
			return "ORDER BY sort ASC, updated DESC"
		}
		return "ORDER BY rank" // 默认是按相关度降序
	default:
		clause := "ORDER BY CASE " +
			"WHEN name = '${keyword}' THEN 10 " +
			"WHEN alias = '${keyword}' THEN 20 " +
			"WHEN name LIKE '%${keyword}%' THEN 50 " +
			"WHEN alias LIKE '%${keyword}%' THEN 60 " +
			"ELSE 65535 END ASC, sort ASC, updated DESC"
		clause = strings.ReplaceAll(clause, "${keyword}", strings.ReplaceAll(query, "'", "''"))
		return clause
	}
}
func removeLimitClause(stmt string) string {
	parsedStmt, err := sqlparser.Parse(stmt)
	if err != nil {
		return stmt
	}

	switch parsedStmt.(type) {
	case *sqlparser.Select:
		slct := parsedStmt.(*sqlparser.Select)
		if nil != slct.Limit {
			slct.Limit = nil
		}
		stmt = sqlparser.String(slct)
	}
	return stmt
}
func stringQuery(query string) string {
	trimmedQuery := strings.TrimSpace(query)
	if "" == trimmedQuery {
		return "\"" + query + "\""
	}

	query = strings.ReplaceAll(query, "\"", "\"\"")
	query = strings.ReplaceAll(query, "'", "''")

	if strings.Contains(trimmedQuery, " ") {
		buf := bytes.Buffer{}
		parts := strings.Split(query, " ")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			part = "\"" + part + "\""
			buf.WriteString(part)
			buf.WriteString(" ")
		}
		return strings.TrimSpace(buf.String())
	}
	return "\"" + query + "\""
}

// markReplaceSpan 用于处理搜索高亮。
func (q *blockSearchRun) fieldRegexp(regexp string) string {
	regexp = strings.ReplaceAll(regexp, "'", "''") // 不需要转义双引号，因为条件都是通过单引号包裹的，只需要转义单引号即可
	buf := bytes.Buffer{}
	buf.WriteString("(")
	buf.WriteString("content REGEXP '")
	buf.WriteString(regexp)
	buf.WriteString("'")
	if q.settings.Name {
		buf.WriteString(" OR name REGEXP '")
		buf.WriteString(regexp)
		buf.WriteString("'")
	}
	if q.settings.Alias {
		buf.WriteString(" OR alias REGEXP '")
		buf.WriteString(regexp)
		buf.WriteString("'")
	}
	if q.settings.Memo {
		buf.WriteString(" OR memo REGEXP '")
		buf.WriteString(regexp)
		buf.WriteString("'")
	}
	if q.settings.IAL {
		buf.WriteString(" OR ial REGEXP '")
		buf.WriteString(regexp)
		buf.WriteString("'")
	}
	buf.WriteString(" OR tag REGEXP '")
	buf.WriteString(regexp)
	buf.WriteString("')")
	return buf.String()
}
func (q *blockSearchRun) columnFilter() string {
	buf := bytes.Buffer{}
	buf.WriteString("{content")
	if q.settings.Name {
		buf.WriteString(" name")
	}
	if q.settings.Alias {
		buf.WriteString(" alias")
	}
	if q.settings.Memo {
		buf.WriteString(" memo")
	}
	if q.settings.IAL {
		buf.WriteString(" ial")
	}
	buf.WriteString(" tag}")
	return buf.String()
}

func SearchColumns(settings conf.Search) string {
	return (&blockSearchRun{settings: settings}).columnFilter()
}
func (q *blockSearchRun) columnConcat() string {
	buf := bytes.Buffer{}
	buf.WriteString("content")
	if q.settings.Name {
		buf.WriteString("||name")
	}
	if q.settings.Alias {
		buf.WriteString("||alias")
	}
	if q.settings.Memo {
		buf.WriteString("||memo")
	}
	if q.settings.IAL {
		buf.WriteString("||ial")
	}
	buf.WriteString("||tag")
	return buf.String()
}
func (q *blockSearchRun) searchBySQL(stmt string, beforeLen, page, pageSize int) (ret []BlockRecord, matchedBlockCount, matchedRootCount int) {
	stmt = strings.TrimSpace(stmt)
	blocks := q.selectBlocks(stmt, page, pageSize)
	ret = blocks
	if 1 > len(ret) {
		ret = []BlockRecord{}
		return
	}

	stmt = strings.ToLower(stmt)
	stdQuery := !strings.Contains(stmt, "with recursive") && !strings.Contains(stmt, "union")
	if stdQuery {
		if strings.HasPrefix(stmt, "select a.* ") { // 多个搜索关键字匹配文档 https://github.com/siyuan-note/siyuan/issues/7350
			stmt = strings.ReplaceAll(stmt, "select a.* ", "select COUNT(a.id) AS `matches`, COUNT(DISTINCT(a.root_id)) AS `docs` ")
		} else {
			stmt = strings.ReplaceAll(stmt, "select * ", "select COUNT(id) AS `matches`, COUNT(DISTINCT(root_id)) AS `docs` ")
		}
	}
	stmt = removeLimitClause(stmt)
	result, _ := q.queryMaps(stmt)
	if 1 > len(result) {
		return
	}

	if !stdQuery {
		var rootIDs, blockIDs []string
		for _, queryResult := range result {
			rootIDs = append(rootIDs, queryResult["root_id"].(string))
			blockIDs = append(blockIDs, queryResult["id"].(string))
		}
		rootIDs = gulu.Str.RemoveDuplicatedElem(rootIDs)
		blockIDs = gulu.Str.RemoveDuplicatedElem(blockIDs)
		matchedRootCount = len(rootIDs)
		matchedBlockCount = len(blockIDs)
	} else {
		matchedBlockCount = int(result[0]["matches"].(int64))
		matchedRootCount = int(result[0]["docs"].(int64))
	}
	return
}
func (q *blockSearchRun) fullTextSearchByRegexp(exp, boxFilter, pathFilter, typeFilter, ignoreFilter, orderBy string, beforeLen, page, pageSize int) (ret []BlockRecord, matchedBlockCount, matchedRootCount int) {
	fieldFilter := q.fieldRegexp(exp)
	stmt := "SELECT * FROM `blocks` WHERE " + fieldFilter + " AND type IN " + typeFilter
	stmt += boxFilter + pathFilter + ignoreFilter + " " + orderBy
	regex, err := regexp.Compile(exp)
	if nil != err {
		q.fail(err)
		return
	}

	blocks := q.selectRegex(stmt, regex, q.settings.Name, q.settings.Alias, q.settings.Memo, q.settings.IAL, page, pageSize)
	ret = blocks
	if 1 > len(ret) {
		ret = []BlockRecord{}
	}

	matchedBlockCount, matchedRootCount = q.fullTextSearchCountByRegexp(exp, boxFilter, pathFilter, typeFilter, ignoreFilter)
	return
}
func (q *blockSearchRun) fullTextSearchCountByRegexp(exp, boxFilter, pathFilter, typeFilter, ignoreFilter string) (matchedBlockCount, matchedRootCount int) {
	fieldFilter := q.fieldRegexp(exp)
	stmt := "SELECT COUNT(id) AS `matches`, COUNT(DISTINCT(root_id)) AS `docs` FROM `blocks` WHERE " + fieldFilter + " AND type IN " + typeFilter + ignoreFilter
	stmt += boxFilter + pathFilter
	result, _ := q.queryMaps(stmt)
	if 1 > len(result) {
		return
	}
	matchedBlockCount = int(result[0]["matches"].(int64))
	matchedRootCount = int(result[0]["docs"].(int64))
	return
}
func (q *blockSearchRun) fullTextSearchByFTS(query, boxFilter, pathFilter, typeFilter, ignoreFilter, orderBy string, beforeLen, page, pageSize int) (ret []BlockRecord, matchedBlockCount, matchedRootCount int) {
	table := "blocks_fts" // 大小写敏感
	if !q.settings.CaseSensitive {
		table = "blocks_fts_case_insensitive"
	}
	projections := "id, parent_id, root_id, hash, box, path, " +
		// Search result content snippet returns more text https://github.com/siyuan-note/siyuan/issues/10707
		"snippet(" + table + ", 6, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS hpath, " +
		"snippet(" + table + ", 7, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS name, " +
		"snippet(" + table + ", 8, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS alias, " +
		"snippet(" + table + ", 9, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS memo, " +
		"snippet(" + table + ", 10, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 64) AS tag, " +
		"snippet(" + table + ", 11, '" + search.SearchMarkLeft + "', '" + search.SearchMarkRight + "', '...', 512) AS content, " +
		"fcontent, markdown, length, type, subtype, ial, sort, created, updated"
	stmt := "SELECT " + projections + " FROM " + table + " WHERE (`" + table + "` MATCH '" + q.columnFilter() + ":(" + query + ")'"
	stmt += ") AND type IN " + typeFilter
	stmt += boxFilter + pathFilter + ignoreFilter + " " + orderBy
	stmt += " LIMIT " + strconv.Itoa(pageSize) + " OFFSET " + strconv.Itoa((page-1)*pageSize)
	blocks := q.selectBlocks(stmt, page, pageSize)
	ret = blocks
	if 1 > len(ret) {
		ret = []BlockRecord{}
	}

	matchedBlockCount, matchedRootCount = q.fullTextSearchCountByFTS(query, boxFilter, pathFilter, typeFilter, ignoreFilter)
	return
}
func (q *blockSearchRun) fullTextSearchCountByFTS(query, boxFilter, pathFilter, typeFilter, ignoreFilter string) (matchedBlockCount, matchedRootCount int) {
	table := "blocks_fts" // 大小写敏感
	if !q.settings.CaseSensitive {
		table = "blocks_fts_case_insensitive"
	}

	stmt := "SELECT COUNT(id) AS `matches`, COUNT(DISTINCT(root_id)) AS `docs` FROM `" + table + "` WHERE (`" + table + "` MATCH '" + q.columnFilter() + ":(" + query + ")'"
	stmt += ") AND type IN " + typeFilter
	stmt += boxFilter + pathFilter + ignoreFilter
	result, _ := q.queryMaps(stmt)
	if 1 > len(result) {
		return
	}
	matchedBlockCount = int(result[0]["matches"].(int64))
	matchedRootCount = int(result[0]["docs"].(int64))
	return
}
func (q *blockSearchRun) fullTextSearchByLikeWithRoot(query, boxFilter, pathFilter, typeFilter, ignoreFilter, orderBy string, beforeLen, page, pageSize int) (ret []BlockRecord, matchedBlockCount, matchedRootCount int) {
	query = strings.ReplaceAll(query, "'", "''") // 不需要转义双引号，因为条件都是通过单引号包裹的，只需要转义单引号即可
	keywords := strings.Split(query, " ")
	contentField := q.columnConcat()
	var likeFilter string
	orderByLike := "("
	for i, keyword := range keywords {
		likeFilter += "GROUP_CONCAT(" + contentField + ") LIKE '%" + keyword + "%'"
		orderByLike += "(docContent LIKE '%" + keyword + "%')"
		if i < len(keywords)-1 {
			likeFilter += " AND "
			orderByLike += " + "
		}
	}
	orderByLike += ")"
	dMatchStmt := "SELECT root_id, MAX(CASE WHEN type = 'd' THEN (" + contentField + ") END) AS docContent" +
		" FROM blocks WHERE type IN " + typeFilter + boxFilter + pathFilter + ignoreFilter +
		" GROUP BY root_id HAVING " + likeFilter + "ORDER BY " + orderByLike + " DESC, MAX(updated) DESC"
	cteStmt := "WITH docBlocks AS (" + dMatchStmt + ")"
	likeFilter = strings.ReplaceAll(likeFilter, "GROUP_CONCAT("+contentField+")", "concatContent")
	limit := " LIMIT " + strconv.Itoa(pageSize) + " OFFSET " + strconv.Itoa((page-1)*pageSize)
	selectStmt := cteStmt + "\nSELECT *, " +
		"(" + contentField + ") AS concatContent, " +
		"(SELECT COUNT(root_id) FROM docBlocks) AS docs, " +
		"(CASE WHEN (root_id IN (SELECT root_id FROM docBlocks) AND (" + strings.ReplaceAll(likeFilter, "concatContent", contentField) + ")) THEN 1 ELSE 0 END) AS blockSort" +
		" FROM blocks WHERE type IN " + typeFilter + boxFilter + pathFilter + ignoreFilter +
		" AND (id IN (SELECT root_id FROM docBlocks " + limit + ") OR" +
		"  (root_id IN (SELECT root_id FROM docBlocks" + limit + ") AND (" + likeFilter + ")))"
	if strings.Contains(orderBy, "ORDER BY rank DESC") {
		orderBy = buildOrderBy(query, 0, 0)
		selectStmt += " " + strings.Replace(orderBy, "END ASC, ", "END ASC, blockSort ASC, ", 1)
	} else if strings.Contains(orderBy, "ORDER BY rank") {
		orderBy = buildOrderBy(query, 0, 0)
		selectStmt += " " + strings.Replace(orderBy, "END ASC, ", "END ASC, blockSort DESC, ", 1)
	} else if strings.Contains(orderBy, "sort ASC") {
		selectStmt += " " + strings.Replace(orderBy, "END ASC, ", "END ASC, blockSort DESC, ", 1)
	} else {
		selectStmt += " " + orderBy
	}
	result, _ := q.queryMaps(selectStmt)
	resultBlocks := q.toBlocks(result)
	if 0 < len(resultBlocks) {
		matchedRootCount = int(result[0]["docs"].(int64))
		matchedBlockCount = matchedRootCount
	}

	keywords = gulu.Str.RemoveDuplicatedElem(keywords)
	terms := strings.Join(keywords, search.TermSep)
	terms = strings.ReplaceAll(terms, "''", "'")
	ret = resultBlocks
	q.terms = terms
	if 1 > len(ret) {
		ret = []BlockRecord{}
	}
	return
}

func (q *blockSearchRun) fail(err error) {
	if q.err == nil && err != nil {
		q.err = err
	}
}
