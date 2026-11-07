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
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"wiki-kernel/internal/indexdb"

	"github.com/siyuan-note/logging"
)

func QueryAssetContentNoLimit(stmt string) (ret []map[string]interface{}, err error) {
	return queryAssetContentRawStmt(stmt, math.MaxInt)
}

func queryAssetContentRawStmt(stmt string, limit int) (ret []map[string]interface{}, err error) {
	rows, err := queryAssetContent(stmt)
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

func SelectAssetContentsRawStmt(stmt string, page, limit int) (ret []*AssetContent) {
	rows, err := indexdb.SelectAssetContentsPage(context.Background(), assetContentDB, stmt, page, limit)
	if err != nil {
		logging.LogWarnf("asset query failed: %s", err)
		return nil
	}
	for i := range rows {
		ret = append(ret, &rows[i])
	}
	return
}

func SelectAssetContentsRawStmtNoParse(stmt string, limit int) (ret []*AssetContent) {
	return selectAssetContentsRawStmt(stmt, limit)
}

func selectAssetContentsRawStmt(stmt string, limit int) (ret []*AssetContent) {
	rows, err := indexdb.SelectAssetContents(context.Background(), assetContentDB, stmt, limit)
	if err != nil && !strings.Contains(err.Error(), "syntax error") {
		logging.LogWarnf("raw asset query failed: %s", err)
	}
	for _, row := range rows {
		value := row
		ret = append(ret, &value)
	}
	return
}

func scanAssetContentRows(rows *sql.Rows) (ret *AssetContent) {
	var ac AssetContent
	if err := rows.Scan(&ac.ID, &ac.Name, &ac.Ext, &ac.Path, &ac.Size, &ac.Updated, &ac.Content); err != nil {
		logging.LogErrorf("query scan field failed: %s\n%s", err, logging.ShortStack())
		return
	}
	ret = &ac
	return
}

func queryAssetContent(query string, args ...interface{}) (*sql.Rows, error) {
	query = strings.TrimSpace(query)
	if "" == query {
		return nil, errors.New("statement is empty")
	}
	if nil == assetContentDB {
		return nil, errors.New("database is nil")
	}
	return assetContentDB.Query(query, args...)
}
