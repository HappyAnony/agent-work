// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"github.com/88250/vitess-sqlparser/sqlparser"
	sqlparser2 "github.com/rqlite/sql"
	"strconv"
	"strings"
)

func mapsLimit(statement sqlparser.Statement, limit int) *sqlparser.Limit {
	var v *sqlparser.Limit
	switch s := statement.(type) {
	case *sqlparser.Select:
		v = s.Limit
	case *sqlparser.Union:
		v = s.Limit
	}
	if v == nil || v.Rowcount == nil {
		return &sqlparser.Limit{Rowcount: &sqlparser.SQLVal{Type: sqlparser.IntVal, Val: []byte(strconv.Itoa(limit))}}
	}
	return v
}
func mapQueryPlan(stmt string, limit int) (string, bool) {
	p := sqlparser2.NewParser(strings.NewReader(stmt))
	statement, err := p.ParseStatement()
	if err == nil {
		if s, ok := statement.(*sqlparser2.SelectStatement); ok {
			if s.LimitExpr == nil {
				s.LimitExpr = &sqlparser2.NumberLit{Value: strconv.Itoa(limit)}
			}
			return s.String(), false
		}
		return stmt, true
	}
	if strings.Contains(stmt, "||") {
		return stmt, true
	}
	statement1, err := sqlparser.Parse(stmt)
	if err != nil {
		return stmt, true
	}
	switch s := statement1.(type) {
	case *sqlparser.Select:
		s.Limit = mapsLimit(s, limit)
	case *sqlparser.Union:
		s.Limit = mapsLimit(s, limit)
	default:
		return stmt, true
	}
	return sqlparser.String(statement1), false
}

// QueryMaps retains the original two-parser normalization, union/concatenation
// fallback and explicit-limit behavior. allowWrites is a trusted compatibility
// index capability; native callers always pass false. Source is never lent.
func QueryMaps(ctx context.Context, db *sql.DB, statement string, limit int, allowWrites bool) (out []map[string]interface{}, err error) {
	if ctx == nil || db == nil {
		return nil, ErrNoDatabase
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if !allowWrites {
		if err = ValidateReadSQL(statement); err != nil {
			return nil, err
		}
	}
	stmt, raw := mapQueryPlan(statement, limit)
	rows, err := Query(ctx, db, stmt)
	if err != nil && !raw {
		rows, err = Query(ctx, db, statement+" LIMIT "+strconv.Itoa(limit))
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, rows.Close(), ctx.Err())
		if err != nil {
			out = nil
		}
	}()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if !raw {
		out = []map[string]interface{}{}
	}
	noLimit := !strings.Contains(strings.ToLower(stmt), " limit ") && !strings.Contains(strings.ToLower(stmt), "\nlimit ") && !strings.Contains(strings.ToLower(stmt), "\tlimit ")
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := map[string]interface{}{}
		for i, column := range columns {
			row[column] = values[i]
		}
		out = append(out, row)
		if raw && noLimit && len(out) > limit {
			break
		}
	}
	err = rows.Err()
	return
}
