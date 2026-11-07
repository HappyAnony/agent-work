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
	"database/sql"
	"errors"
	"strings"
	"wiki-kernel/internal/indexdb"

	"github.com/siyuan-note/logging"
)

type History = indexdb.HistoryEntry

func QueryHistory(stmt string) (ret []map[string]interface{}, err error) {
	ret = []map[string]interface{}{}
	rows, err := queryHistory(stmt)
	if err != nil {
		logging.LogWarnf("sql query [%s] failed: %s", stmt, err)
		return
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	if nil == cols {
		return
	}

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
	}
	return
}

func SelectHistoriesRawStmt(stmt string) (ret []*History) {
	if nil == historyDB {
		return
	}
	rows, err := historyDB.Query(stmt)
	if err != nil {
		logging.LogWarnf("sql query [%s] failed: %s", stmt, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		if history := scanHistoryRows(rows); nil != history {
			ret = append(ret, history)
		}
	}
	return
}

func scanHistoryRows(rows *sql.Rows) (ret *History) {
	var history History
	if err := rows.Scan(&history.ID, &history.Type, &history.Op, &history.Title, &history.Content, &history.Path, &history.Created); err != nil {
		logging.LogErrorf("query scan field failed: %s\n%s", err, logging.ShortStack())
		return
	}
	ret = &history
	return
}

func queryHistory(query string, args ...interface{}) (*sql.Rows, error) {
	query = strings.TrimSpace(query)
	if "" == query {
		return nil, errors.New("statement is empty")
	}
	if nil == historyDB {
		return nil, errors.New("database is nil")
	}
	return historyDB.Query(query, args...)
}
