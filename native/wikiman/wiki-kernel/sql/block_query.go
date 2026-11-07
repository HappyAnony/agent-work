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

package sql

import (
	"bytes"
	"context"
	"database/sql"
	"math"
	"regexp"
	"strconv"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"

	"github.com/88250/lute/ast"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/siyuan-note/logging"
)

func QueryEmptyContentEmbedBlocks() (ret []*Block) {
	rows, err := indexdb.EmptyEmbedBlocks(context.Background(), db)
	if err != nil {
		logging.LogErrorf("query empty embeds failed: %s", err)
	}
	for _, row := range rows {
		block := Block(row)
		putBlockCache(&block)
		ret = append(ret, &block)
	}
	return
}

// SearchDocumentRoots keeps legacy error logging/empty-query fallback.
func SearchDocumentRoots(ctx context.Context, settings conf.Search, words, exclude []string) []indexdb.BlockRecord {
	ret, err := indexdb.SearchDocumentRoots(ctx, db, settings, words, exclude)
	if err != nil {
		logging.LogErrorf("document root search failed: %s", err)
		return nil
	}
	return ret
}

func QueryRootBlockByCondition(condition string, limit int) (ret []*Block) {
	sqlStmt := "SELECT *, length(hpath) - length(replace(hpath, '/', '')) AS lv FROM blocks WHERE type = 'd' AND " + condition + " ORDER BY box DESC,lv ASC LIMIT " + strconv.Itoa(limit)
	rows, err := query(sqlStmt)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var block Block
		var sepCount int
		if err = rows.Scan(&block.ID, &block.ParentID, &block.RootID, &block.Hash, &block.Box, &block.Path, &block.HPath, &block.Name, &block.Alias, &block.Memo, &block.Tag, &block.Content, &block.FContent, &block.Markdown, &block.Length, &block.Type, &block.SubType, &block.IAL, &block.Sort, &block.Created, &block.Updated, &sepCount); err != nil {
			logging.LogErrorf("query scan field failed: %s", err)
			return
		}
		ret = append(ret, &block)
	}
	return
}

func (block *Block) IsContainerBlock() bool {
	switch block.Type {
	case "d", "b", "l", "i", "s":
		return true
	}
	return false
}

func queryBlockChildrenIDs(id string) (ret []string) {
	ret, err := (indexdb.References{DB: db}).BlockChildren(context.Background(), id)
	if err != nil {
		logging.LogErrorf("block children failed: %s", err)
	}
	return ret
}

func queryBlockIDByParentID(parentID string) (ret []string) {
	sqlStmt := "SELECT id FROM blocks WHERE parent_id = ?"
	rows, err := query(sqlStmt, parentID)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ret = append(ret, id)
	}
	return
}

func QueryBlockAliases(rootID string) (ret []string) {
	sqlStmt := "SELECT alias FROM blocks WHERE root_id = ? AND alias != ''"
	rows, err := query(sqlStmt, rootID)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()
	var aliasesRows []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		aliasesRows = append(aliasesRows, name)
	}

	for _, aliasStr := range aliasesRows {
		aliases := strings.Split(aliasStr, ",")
		for _, alias := range aliases {
			var exist bool
			for _, retAlias := range ret {
				if retAlias == alias {
					exist = true
				}
			}
			if !exist {
				ret = append(ret, alias)
			}
		}
	}
	return
}

func queryNames(searchIgnoreLines []string) (ret []string) {
	ret = []string{}
	sqlStmt := "SELECT name FROM blocks WHERE name != ''"
	buf := bytes.Buffer{}
	for _, line := range searchIgnoreLines {
		buf.WriteString(" AND ")
		buf.WriteString(line)
	}
	sqlStmt += buf.String()
	sqlStmt += " LIMIT ?"
	rows, err := query(sqlStmt, 10240)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()

	var namesRows []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		namesRows = append(namesRows, name)
	}

	set := hashset.New()
	for _, namesStr := range namesRows {
		names := strings.Split(namesStr, ",")
		for _, name := range names {
			if "" == strings.TrimSpace(name) {
				continue
			}
			set.Add(name)
		}
	}
	for _, v := range set.Values() {
		ret = append(ret, v.(string))
	}
	return
}

func queryAliases(searchIgnoreLines []string) (ret []string) {
	ret = []string{}
	sqlStmt := "SELECT alias FROM blocks WHERE alias != ''"
	buf := bytes.Buffer{}
	for _, line := range searchIgnoreLines {
		buf.WriteString(" AND ")
		buf.WriteString(line)
	}
	sqlStmt += buf.String()
	sqlStmt += " LIMIT ?"
	rows, err := query(sqlStmt, 10240)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()

	var aliasesRows []string
	for rows.Next() {
		var alias string
		rows.Scan(&alias)
		aliasesRows = append(aliasesRows, alias)
	}

	set := hashset.New()
	for _, aliasStr := range aliasesRows {
		aliases := strings.Split(aliasStr, ",")
		for _, alias := range aliases {
			if "" == strings.TrimSpace(alias) {
				continue
			}
			set.Add(alias)
		}
	}
	for _, v := range set.Values() {
		ret = append(ret, v.(string))
	}
	return
}

func queryDocIDsByTitle(title string, excludeIDs []string) (ret []string) {
	ret = []string{}
	notIn := "('" + strings.Join(excludeIDs, "','") + "')"

	sqlStmt := "SELECT id FROM blocks WHERE type = 'd' AND content LIKE ? AND id NOT IN " + notIn + " LIMIT ?"
	if caseSensitive {
		sqlStmt = "SELECT id FROM blocks WHERE type = 'd' AND content = ? AND id NOT IN " + notIn + " LIMIT ?"
	}
	rows, err := query(sqlStmt, title, 32)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()

	set := hashset.New()
	for rows.Next() {
		var id string
		rows.Scan(&id)
		set.Add(id)
	}

	for _, v := range set.Values() {
		ret = append(ret, v.(string))
	}
	return
}

func queryDocTitles(searchIgnoreLines []string) (ret []string) {
	ret = []string{}
	sqlStmt := "SELECT content FROM blocks WHERE type = 'd'"
	buf := bytes.Buffer{}
	for _, line := range searchIgnoreLines {
		buf.WriteString(" AND ")
		buf.WriteString(line)
	}
	sqlStmt += buf.String()
	rows, err := query(sqlStmt)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()

	var docNamesRows []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		docNamesRows = append(docNamesRows, name)
	}

	set := hashset.New()
	for _, nameStr := range docNamesRows {
		names := strings.Split(nameStr, ",")
		for _, name := range names {
			if "" == strings.TrimSpace(name) {
				continue
			}
			set.Add(name)
		}
	}
	for _, v := range set.Values() {
		ret = append(ret, v.(string))
	}
	return
}

func QueryBlockNamesByRootID(rootID string) (ret []string) {
	sqlStmt := "SELECT DISTINCT name FROM blocks WHERE root_id = ? AND name != ''"
	rows, err := query(sqlStmt, rootID)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		rows.Scan(&name)
		ret = append(ret, name)
	}
	return
}

// The original keyword argument deliberately does not filter SQL candidates.
// Rename/remove compare the current tree attribute exactly.
func QueryBookmarkBlocksByKeyword(bookmark string) []*Block { return QueryBookmarkBlocks() }

func QueryBookmarkBlocks() (ret []*Block) {
	rows, err := indexdb.BookmarkBlocks(context.Background(), db)
	if err != nil {
		logging.LogErrorf("bookmark query failed: %s", err)
		return nil
	}
	for _, row := range rows {
		b := Block(row)
		ret = append(ret, &b)
	}
	return
}

func QueryBookmarkLabels() []string {
	ret, err := indexdb.BookmarkLabels(context.Background(), db)
	if err != nil {
		logging.LogErrorf("bookmark labels failed: %s", err)
		return []string{}
	}
	return ret
}

func QueryNoLimit(stmt string) (ret []map[string]interface{}, err error) {
	return queryRawStmt(stmt, math.MaxInt)
}

func Query(stmt string, limit int) ([]map[string]interface{}, error) {
	return indexdb.QueryMaps(context.Background(), db, stmt, limit, true)
}

func ToBlocks(result []map[string]interface{}) (ret []*Block) {
	for _, row := range result {
		b := &Block{
			ID:       row["id"].(string),
			ParentID: row["parent_id"].(string),
			RootID:   row["root_id"].(string),
			Hash:     row["hash"].(string),
			Box:      row["box"].(string),
			Path:     row["path"].(string),
			HPath:    row["hpath"].(string),
			Name:     row["name"].(string),
			Alias:    row["alias"].(string),
			Memo:     row["memo"].(string),
			Tag:      row["tag"].(string),
			Content:  row["content"].(string),
			FContent: row["fcontent"].(string),
			Markdown: row["markdown"].(string),
			Length:   int(row["length"].(int64)),
			Type:     row["type"].(string),
			SubType:  row["subtype"].(string),
			IAL:      row["ial"].(string),
			Sort:     int(row["sort"].(int64)),
			Created:  row["created"].(string),
			Updated:  row["updated"].(string),
		}
		ret = append(ret, b)
	}
	return
}

func queryRawStmt(stmt string, limit int) (ret []map[string]interface{}, err error) {
	rows, err := query(stmt)
	if err != nil {
		if strings.Contains(err.Error(), "syntax error") {
			return
		}
		return
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil || nil == cols {
		return
	}

	noLimit := !containsLimitClause(stmt)
	var count, errCount int
	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err = rows.Scan(columnPointers...); err != nil {
			return
		}

		m := make(map[string]interface{})
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			m[colName] = *val
		}

		ret = append(ret, m)
		count++
		if (noLimit && limit < count) || 0 < errCount {
			break
		}
	}
	return
}

func SelectBlocksRawStmtNoParse(stmt string, limit int) (ret []*Block) {
	return selectBlocksRawStmt(stmt, limit)
}

func SelectBlocksRawStmt(stmt string, page, limit int) (ret []*Block) {
	rows, err := indexdb.SelectBlockRows(context.Background(), db, stmt, page, limit)
	if err != nil {
		logging.LogWarnf("select blocks failed: %s", err)
		return
	}
	for _, r := range rows {
		b := Block(r)
		ret = append(ret, &b)
	}
	return
}

func SelectBlocksRegex(stmt string, exp *regexp.Regexp, name, alias, memo, ial bool, page, pageSize int) (ret []*Block) {
	rows, err := query(stmt)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", stmt, err)
		return
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		if count <= (page-1)*pageSize {
			continue
		}

		var block Block
		if err := rows.Scan(&block.ID, &block.ParentID, &block.RootID, &block.Hash, &block.Box, &block.Path, &block.HPath, &block.Name, &block.Alias, &block.Memo, &block.Tag, &block.Content, &block.FContent, &block.Markdown, &block.Length, &block.Type, &block.SubType, &block.IAL, &block.Sort, &block.Created, &block.Updated); err != nil {
			logging.LogErrorf("query scan field failed: %s\n%s", err, logging.ShortStack())
			return
		}

		hitContent := exp.MatchString(block.Content)
		hitName := name && exp.MatchString(block.Name)
		hitAlias := alias && exp.MatchString(block.Alias)
		hitMemo := memo && exp.MatchString(block.Memo)
		hitIAL := ial && exp.MatchString(block.IAL)
		if hitContent || hitName || hitAlias || hitMemo || hitIAL {
			if hitContent {
				block.Content = exp.ReplaceAllString(block.Content, "__@mark__${0}__mark@__")
			}
			if hitName {
				block.Name = exp.ReplaceAllString(block.Name, "__@mark__${0}__mark@__")
			}
			if hitAlias {
				block.Alias = exp.ReplaceAllString(block.Alias, "__@mark__${0}__mark@__")
			}
			if hitMemo {
				block.Memo = exp.ReplaceAllString(block.Memo, "__@mark__${0}__mark@__")
			}
			if hitIAL {
				block.IAL = exp.ReplaceAllString(block.IAL, "__@mark__${0}__mark@__")
			}

			ret = append(ret, &block)
			if len(ret) >= pageSize {
				break
			}
		}
	}
	return
}

func selectBlocksRawStmt(stmt string, limit int) (ret []*Block) {
	rows, err := query(stmt)
	if err != nil {
		if strings.Contains(err.Error(), "syntax error") {
			return
		}
		return
	}
	defer rows.Close()

	noLimit := !containsLimitClause(stmt)
	var count, errCount int
	for rows.Next() {
		count++
		if block := scanBlockRows(rows); nil != block {
			ret = append(ret, block)
		} else {
			logging.LogWarnf("raw sql query [%s] failed", stmt)
			errCount++
		}

		if (noLimit && limit < count) || 0 < errCount {
			break
		}
	}
	return
}

func scanBlockRows(rows *sql.Rows) (ret *Block) {
	var block Block
	if err := rows.Scan(&block.ID, &block.ParentID, &block.RootID, &block.Hash, &block.Box, &block.Path, &block.HPath, &block.Name, &block.Alias, &block.Memo, &block.Tag, &block.Content, &block.FContent, &block.Markdown, &block.Length, &block.Type, &block.SubType, &block.IAL, &block.Sort, &block.Created, &block.Updated); err != nil {
		logging.LogErrorf("query scan field failed: %s\n%s", err, logging.ShortStack())
		return
	}
	ret = &block
	putBlockCache(ret)
	return
}

func scanBlockRow(row *sql.Row) (ret *Block) {
	var block Block
	if err := row.Scan(&block.ID, &block.ParentID, &block.RootID, &block.Hash, &block.Box, &block.Path, &block.HPath, &block.Name, &block.Alias, &block.Memo, &block.Tag, &block.Content, &block.FContent, &block.Markdown, &block.Length, &block.Type, &block.SubType, &block.IAL, &block.Sort, &block.Created, &block.Updated); err != nil {
		if sql.ErrNoRows != err {
			logging.LogErrorf("query scan field failed: %s\n%s", err, logging.ShortStack())
		}
		return
	}
	ret = &block
	putBlockCache(ret)
	return
}

func GetChildBlocks(parentID, condition string, limit int) (ret []*Block) {
	blockIDs := queryBlockChildrenIDs(parentID)
	var params []string
	for _, id := range blockIDs {
		params = append(params, "\""+id+"\"")
	}

	ret = []*Block{}
	sqlStmt := "SELECT * FROM blocks AS ref WHERE ref.id IN (" + strings.Join(params, ",") + ")"
	if "" != condition {
		sqlStmt += " AND " + condition
	}
	sqlStmt += " LIMIT " + strconv.Itoa(limit)
	rows, err := query(sqlStmt)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		if block := scanBlockRows(rows); nil != block {
			ret = append(ret, block)
		}
	}
	return
}

func GetAllChildBlocks(rootIDs []string, condition string, limit int) (ret []*Block) {
	ret = []*Block{}
	sqlStmt := "SELECT * FROM blocks AS ref WHERE ref.root_id IN ('" + strings.Join(rootIDs, "','") + "')"
	if "" != condition {
		sqlStmt += " AND " + condition
	}
	sqlStmt += " LIMIT " + strconv.Itoa(limit)
	rows, err := query(sqlStmt)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", sqlStmt, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		if block := scanBlockRows(rows); nil != block {
			ret = append(ret, block)
		}
	}
	return
}

func GetBlock(id string) *Block {
	if db == nil {
		return nil
	}
	row, ok, e := legacyCachedBlocks().Get(context.Background(), id)
	if e != nil {
		logging.LogErrorf("get block: %s", e)
		return nil
	}
	if !ok {
		return nil
	}
	ret := Block(row)
	return &ret
}

func GetRootUpdated() (map[string]string, error) {
	return CaptureLegacyRepairQueries().RootUpdated(context.Background())
}
func GetDuplicatedRootIDs(table string) (ret []string) {
	ret, e := CaptureLegacyRepairQueries().DuplicatedRoots(context.Background(), table)
	if e != nil {
		logging.LogErrorf("duplicate root query failed: %s", e)
	}
	return ret
}

func GetAllRootBlocks() (ret []*Block) {
	stmt := "SELECT * FROM blocks WHERE type = 'd'"
	rows, err := query(stmt)
	if err != nil {
		logging.LogErrorf("sql query [%s] failed: %s", stmt, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		if block := scanBlockRows(rows); nil != block {
			ret = append(ret, block)
		}
	}
	return
}

func GetBlocks(ids []string) (ret []*Block) {
	if db == nil {
		return nil
	}
	rows, e := legacyCachedBlocks().GetMany(context.Background(), ids)
	if e != nil {
		logging.LogErrorf("get blocks: %s", e)
		return nil
	}
	for _, row := range rows {
		if row == nil {
			ret = append(ret, nil)
		} else {
			copy := Block(*row)
			ret = append(ret, &copy)
		}
	}
	return
}

func GetContainerText(container *ast.Node) string {
	text, err := indexdb.ContainerText(context.Background(), container, func(ctx context.Context, id string) (string, error) { return GetRefText(id), nil })
	if err != nil {
		logging.LogErrorf("container text failed: %s", err)
	}
	return text
}

func containsLimitClause(stmt string) bool {
	return strings.Contains(strings.ToLower(stmt), " limit ") ||
		strings.Contains(strings.ToLower(stmt), "\nlimit ") ||
		strings.Contains(strings.ToLower(stmt), "\tlimit ")
}

func SearchBlocks(ctx context.Context, settings conf.Search, request indexdb.BlockSearchRequest) (indexdb.BlockSearchResult, error) {
	return indexdb.SearchBlocks(ctx, db, settings, request)
}

func SearchAllBlocks(ctx context.Context, settings conf.Search, request indexdb.BlockSearchRequest) (indexdb.BlockSearchResult, error) {
	return indexdb.SearchAllBlocks(ctx, db, settings, request)
}
