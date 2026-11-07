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
	"strings"
	"wiki-kernel/internal/indexdb"

	"github.com/siyuan-note/logging"
	"wiki-kernel/search"
)

func GetRefDuplicatedDefRootIDs() (ret []string) {
	ret, err := (indexdb.References{DB: db}).DuplicatedDefinitionRoots(context.Background())
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
	}
	return ret
}

func QueryVirtualRefKeywords(name, alias, anchor, doc bool, searchIgnoreLines, refSearchIgnoreLines []string) (ret []string) {
	ret, err := (indexdb.References{DB: db}).VirtualKeywords(context.Background(), name, alias, anchor, doc, searchIgnoreLines, refSearchIgnoreLines)
	if err != nil {
		logging.LogErrorf("reference keywords failed: %s", err)
	}
	return ret
}

func queryRefTexts(ignore []string) (ret []string) {
	ret, err := (indexdb.References{DB: db}).VirtualKeywords(context.Background(), false, false, true, false, nil, ignore)
	if err != nil {
		logging.LogErrorf("reference keywords failed: %s", err)
	}
	return ret
}

func QueryRefCount(defIDs []string) (ret map[string]int) {
	ret, err := (indexdb.References{DB: db}).DefinitionCounts(context.Background(), defIDs)
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
	}
	return ret
}

func QueryRootChildrenRefCount(defRootID string) (ret map[string]int) {
	ret, err := (indexdb.References{DB: db}).RootChildrenCounts(context.Background(), defRootID)
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
	}
	return ret
}

func QueryRootBlockRefCount() (ret map[string]int) {
	ret, err := (indexdb.References{DB: db}).RootCounts(context.Background())
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
	}
	return ret
}

func QueryDefRootBlocksByRefRootID(refRootID string) (ret []*Block) {
	rows, err := (indexdb.References{DB: db}).DefinitionRoots(context.Background(), refRootID)
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
		return
	}
	for _, v := range rows {
		v := Block(v)
		ret = append(ret, &v)
	}
	return
}

func QueryRefRootBlocksByDefRootIDs(ids []string) (ret map[string][]*Block) {
	rows, err := (indexdb.References{DB: db}).ReferringRoots(context.Background(), ids)
	if err != nil {
		logging.LogErrorf("reference roots failed: %s", err)
	}
	ret = map[string][]*Block{}
	for id, blocks := range rows {
		for _, v := range blocks {
			b := Block(v)
			ret[id] = append(ret[id], &b)
		}
	}
	return
}

func GetRefText(defBlockID string) (ret string) {
	ret = getRefText(defBlockID)
	ret = strings.ReplaceAll(ret, search.SearchMarkLeft, "")
	ret = strings.ReplaceAll(ret, search.SearchMarkRight, "")
	return
}

func getRefText(defBlockID string) string {
	text, err := indexdb.ReferenceText(context.Background(), defBlockID, legacyTreeProjection().parser, func(ctx context.Context, id string) (indexdb.BlockRecord, bool, error) {
		b := GetBlock(id)
		if b == nil {
			return indexdb.BlockRecord{}, false, nil
		}
		return indexdb.BlockRecord(*b), true, nil
	})
	if err != nil {
		logging.LogErrorf("reference text failed: %s", err)
	}
	return text
}

func QueryBlockDefIDsByRefText(text string, exclude []string) (ret []string) {
	ret, err := (indexdb.References{DB: db}).DefinitionIDsByText(context.Background(), text, exclude, caseSensitive)
	if err != nil {
		logging.LogErrorf("reference match failed: %s", err)
	}
	return ret
}

func QueryChildRefDefIDsByRootDefID(rootDefID string) (ret map[string][]string) {
	rows, err := (indexdb.References{DB: db}).ChildDefinitions(context.Background(), rootDefID)
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
		return
	}
	return rows
}

func QueryChildDefIDsByRootDefID(rootDefID string) (ret []string) {
	rows, err := (indexdb.References{DB: db}).ChildDefinitionIDs(context.Background(), rootDefID)
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
		return
	}
	return rows
}

func QueryRefIDsByDefID(defID string, containChildren bool) (ret []string) {
	rows, err := (indexdb.References{DB: db}).RefIDs(context.Background(), defID, containChildren)
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
		return
	}
	return rows
}

func QueryRefsRecent(onlyDoc bool, typeFilter string, ignore []string) (ret []*Ref) {
	rows, err := (indexdb.References{DB: db}).Recent(context.Background(), onlyDoc, typeFilter, ignore)
	if err != nil {
		logging.LogErrorf("recent references failed: %s", err)
		return
	}
	for _, r := range rows {
		r := r
		ret = append(ret, &r)
	}
	return
}

func QueryRefsByDefID(defBlockID string, containChildren bool) (ret []*Ref) {
	rows, err := (indexdb.References{DB: db}).ByDefinition(context.Background(), defBlockID, containChildren)
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
		return
	}
	for _, v := range rows {
		v := v
		ret = append(ret, &v)
	}
	return
}

func QueryRefsByDefIDRefID(defBlockID, refBlockID string) (ret []*Ref) {
	rows, err := (indexdb.References{DB: db}).ByPair(context.Background(), defBlockID, refBlockID)
	if err != nil {
		logging.LogErrorf("reference query failed: %s", err)
		return
	}
	for _, v := range rows {
		v := v
		ret = append(ret, &v)
	}
	return
}

func DefRefs(condition string, limit int) (ret []map[*Block]*Block) {
	rows, err := (indexdb.References{DB: db}).DefinitionPairs(context.Background(), condition, limit)
	if err != nil {
		logging.LogErrorf("reference graph failed: %s", err)
		return
	}
	ret = []map[*Block]*Block{}
	defs := map[string]*Block{}
	for _, pair := range rows {
		def := defs[pair.Definition.ID]
		if def == nil {
			b := Block(pair.Definition)
			def = &b
			defs[b.ID] = def
		}
		ref := Block(pair.Reference)
		ret = append(ret, map[*Block]*Block{def: &ref})
	}
	return
}

func scanRefRows(rows *sql.Rows) (ret *Ref) {
	var ref Ref
	if err := rows.Scan(&ref.ID, &ref.DefBlockID, &ref.DefBlockParentID, &ref.DefBlockRootID, &ref.DefBlockPath, &ref.BlockID, &ref.RootID, &ref.Box, &ref.Path, &ref.Content, &ref.Markdown, &ref.Type); err != nil {
		logging.LogErrorf("query scan field failed: %s", err)
		return
	}
	ret = &ref
	return
}
