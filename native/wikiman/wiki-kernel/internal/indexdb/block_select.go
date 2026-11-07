// SPDX-License-Identifier: AGPL-3.0-or-later
// Original SQL block selectors; explicit DB/context, no global cache/logging.
package indexdb

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/88250/vitess-sqlparser/sqlparser"
	"regexp"
	"strconv"
	"strings"
)

func SelectBlockRows(ctx context.Context, db *sql.DB, stmt string, page, limit int) ([]BlockRecord, error) {
	q := &blockSearchRun{ctx: ctx, db: db}
	rows := q.selectBlocks(stmt, page, limit)
	return rows, q.err
}

// SelectReadBlockRows keeps the inherited selection/pagination algorithm while
// validating both the input and any rewritten query as a single read statement.
func SelectReadBlockRows(ctx context.Context, db *sql.DB, stmt string, page, limit int) ([]BlockRecord, error) {
	if ctx == nil || db == nil {
		return nil, ErrNoDatabase
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateReadSQL(stmt); err != nil {
		return nil, err
	}
	q := &blockSearchRun{ctx: ctx, db: db, strictRead: true}
	rows := q.selectBlocks(stmt, page, limit)
	return rows, q.err
}
func (q *blockSearchRun) selectBlocks(stmt string, page, limit int) (ret []BlockRecord) {
	parsedStmt, err := sqlparser.Parse(stmt)
	if err != nil {
		return q.selectRaw(stmt, limit)
	}

	switch parsedStmt.(type) {
	case *sqlparser.Select:
		slct := parsedStmt.(*sqlparser.Select)
		if nil == slct.Limit {
			slct.Limit = &sqlparser.Limit{
				Rowcount: &sqlparser.SQLVal{
					Type: sqlparser.IntVal,
					Val:  []byte(strconv.Itoa(limit)),
				},
			}
			slct.Limit.Offset = &sqlparser.SQLVal{
				Type: sqlparser.IntVal,
				Val:  []byte(strconv.Itoa((page - 1) * limit)),
			}
		} else {
			if nil != slct.Limit.Rowcount && 0 < len(slct.Limit.Rowcount.(*sqlparser.SQLVal).Val) {
				limit, _ = strconv.Atoi(string(slct.Limit.Rowcount.(*sqlparser.SQLVal).Val))
				if 0 >= limit {
					limit = 32
				}
			}

			slct.Limit.Rowcount = &sqlparser.SQLVal{
				Type: sqlparser.IntVal,
				Val:  []byte(strconv.Itoa(limit)),
			}
			slct.Limit.Offset = &sqlparser.SQLVal{
				Type: sqlparser.IntVal,
				Val:  []byte(strconv.Itoa((page - 1) * limit)),
			}
		}

		stmt = sqlparser.String(slct)
	default:
		return
	}

	stmt = strings.ReplaceAll(stmt, "\\'", "''")
	stmt = strings.ReplaceAll(stmt, "\\\"", "\"")
	stmt = strings.ReplaceAll(stmt, "\\\\*", "\\*")
	stmt = strings.ReplaceAll(stmt, "from dual", "")
	rows, err := q.query(stmt)
	if err != nil {
		q.fail(err)
		if strings.Contains(err.Error(), "syntax error") {
			return
		}
		q.fail(err)
		return
	}
	defer rows.Close()
	defer func() { q.fail(rows.Err()) }()
	for rows.Next() {
		if block := q.scanBlock(rows); nil != block {
			ret = append(ret, *block)
		}
	}
	return
}
func (q *blockSearchRun) selectRegex(stmt string, exp *regexp.Regexp, name, alias, memo, ial bool, page, pageSize int) (ret []BlockRecord) {
	rows, err := q.query(stmt)
	if err != nil {
		q.fail(err)
		return
	}
	defer rows.Close()
	defer func() { q.fail(rows.Err()) }()
	count := 0
	for rows.Next() {
		count++
		if count <= (page-1)*pageSize {
			continue
		}

		var block BlockRecord
		if err := rows.Scan(&block.ID, &block.ParentID, &block.RootID, &block.Hash, &block.Box, &block.Path, &block.HPath, &block.Name, &block.Alias, &block.Memo, &block.Tag, &block.Content, &block.FContent, &block.Markdown, &block.Length, &block.Type, &block.SubType, &block.IAL, &block.Sort, &block.Created, &block.Updated); err != nil {
			q.fail(err)
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

			ret = append(ret, block)
			if len(ret) >= pageSize {
				break
			}
		}
	}
	return
}
func (q *blockSearchRun) selectRaw(stmt string, limit int) (ret []BlockRecord) {
	rows, err := q.query(stmt)
	if err != nil {
		q.fail(err)
		if strings.Contains(err.Error(), "syntax error") {
			return
		}
		return
	}
	defer rows.Close()
	defer func() { q.fail(rows.Err()) }()

	noLimit := !containsSearchLimit(stmt)
	var count, errCount int
	for rows.Next() {
		count++
		if block := q.scanBlock(rows); nil != block {
			ret = append(ret, *block)
		} else {
			q.fail(fmt.Errorf("invalid_block_row"))
			errCount++
		}

		if (noLimit && limit < count) || 0 < errCount {
			break
		}
	}
	return
}
func (q *blockSearchRun) queryRawMaps(stmt string, limit int) (ret []map[string]interface{}, err error) {
	rows, err := q.query(stmt)
	if err != nil {
		if strings.Contains(err.Error(), "syntax error") {
			q.fail(err)
			return
		}
		return
	}
	defer rows.Close()
	defer func() { q.fail(rows.Err()) }()

	cols, err := rows.Columns()
	if err != nil || nil == cols {
		return
	}

	noLimit := !containsSearchLimit(stmt)
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
func (q *blockSearchRun) toBlocks(result []map[string]interface{}) (ret []BlockRecord) {
	defer func() {
		if v := recover(); v != nil {
			q.fail(fmt.Errorf("invalid_block_projection: %T", v))
			ret = nil
		}
	}()
	for _, row := range result {
		b := BlockRecord{
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

func (q *blockSearchRun) scanBlock(rows *sql.Rows) *BlockRecord {
	b, e := scanBlock(rows)
	if e != nil {
		q.fail(e)
		return nil
	}
	return &b
}
func (q *blockSearchRun) queryMaps(stmt string) ([]map[string]interface{}, error) {
	rows, e := q.queryRawMaps(stmt, int(^uint(0)>>1))
	q.fail(e)
	return rows, e
}

func (q *blockSearchRun) query(stmt string) (*sql.Rows, error) {
	if q.strictRead {
		if err := ValidateReadSQL(stmt); err != nil {
			q.fail(err)
			return nil, err
		}
	}
	return Query(q.ctx, q.db, stmt)
}
func containsSearchLimit(stmt string) bool {
	return strings.Contains(strings.ToLower(stmt), " limit ") || strings.Contains(strings.ToLower(stmt), "\nlimit ") || strings.Contains(strings.ToLower(stmt), "\tlimit ")
}
