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
	"wiki-kernel/internal/indexdb"

	"github.com/88250/lute/parse"
)

type Ref = indexdb.RefRecord

func upsertRefs(tx *sql.Tx, tree *parse.Tree) error {
	refs, annotations, err := legacyTreeProjection().referenceRows(tree)
	if err != nil {
		return err
	}
	if err = indexdb.ReplaceReferences(nativeSQLContext(), tx, tree.Box, tree.Path, refs, annotations); err != nil {
		return err
	}
	for i := range refs {
		putRefCache(&refs[i])
	}
	return nil
}

func deleteRefs(tx *sql.Tx, tree *parse.Tree) (err error) {
	return indexdb.ApplyMainMutation(nativeSQLContext(), tx, indexdb.MainMutation{Action: "delete_refs", ID: tree.ID, Box: tree.Box, Path: tree.Path}, caseSensitive)

}

func insertRefs(tx *sql.Tx, tree *parse.Tree) (err error) {
	refs, fileAnnotationRefs, err := legacyTreeProjection().referencePointers(tree)
	if err != nil {
		return err
	}
	if err = insertBlockRefs(tx, refs); err != nil {
		return
	}
	if err = insertFileAnnotationRefs(tx, fileAnnotationRefs); err != nil {
		return
	}
	return err
}
