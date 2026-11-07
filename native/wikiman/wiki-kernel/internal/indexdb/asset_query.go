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

package indexdb

import (
	"context"
	"database/sql"
	"strings"
)

// SelectAssetContents is the original typed asset-row selection algorithm,
// including its existing limit/no-limit behavior. It is internal, not a new
// public arbitrary-SQL handler or an authoritative source reader.
func SelectAssetContents(ctx context.Context, db *sql.DB, statement string, limit int) (ret []AssetContentEntry, err error) {
	rows, err := Query(ctx, db, statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lower := strings.ToLower(statement)
	noLimit := !strings.Contains(lower, " limit ") && !strings.Contains(lower, "\nlimit ") && !strings.Contains(lower, "\tlimit ")
	count := 0
	for rows.Next() {
		count++
		var a AssetContentEntry
		if err = rows.Scan(&a.ID, &a.Name, &a.Ext, &a.Path, &a.Size, &a.Updated, &a.Content); err != nil {
			return ret, err
		}
		ret = append(ret, a)
		if noLimit && limit < count {
			break
		}
	}
	return ret, rows.Err()
}
