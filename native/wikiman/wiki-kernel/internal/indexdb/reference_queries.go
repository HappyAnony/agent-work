// SPDX-License-Identifier: AGPL-3.0-or-later
// Original sql/block_ref_query.go algorithms with explicit DB/cancellation.
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

func scanReference(rows *sql.Rows) (r RefRecord, err error) {
	err = rows.Scan(&r.ID, &r.DefBlockID, &r.DefBlockParentID, &r.DefBlockRootID, &r.DefBlockPath, &r.BlockID, &r.RootID, &r.Box, &r.Path, &r.Content, &r.Markdown, &r.Type)
	return
}
func (q References) queryRefs(ctx context.Context, stmt string, args ...interface{}) (ret []RefRecord, err error) {
	rows, err := Query(ctx, q.DB, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		r, e := scanReference(rows)
		if e != nil {
			return nil, e
		}
		ret = append(ret, r)
	}
	return ret, rows.Err()
}

// ChildDefinitions maps each referencing block ID to its definition IDs in the
// requested root. The original backlink promotion/count code consumes this
// direction; a definition-to-reference map incorrectly counts definitions.
func (q References) ChildDefinitions(ctx context.Context, root string) (map[string][]string, error) {
	rows, err := Query(ctx, q.DB, "SELECT block_id,def_block_id FROM refs WHERE def_block_root_id=?", root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ret := map[string][]string{}
	for rows.Next() {
		var ref, def string
		if err = rows.Scan(&ref, &def); err != nil {
			return nil, err
		}
		ret[ref] = append(ret[ref], def)
	}
	return ret, rows.Err()
}
func (q References) ids(ctx context.Context, stmt string, args ...interface{}) (ret []string, err error) {
	rows, err := Query(ctx, q.DB, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ret = append(ret, id)
	}
	return ret, rows.Err()
}
func (q References) ChildDefinitionIDs(ctx context.Context, root string) ([]string, error) {
	return q.ids(ctx, "SELECT DISTINCT def_block_id FROM refs WHERE def_block_root_id=?", root)
}
func (q References) RefIDs(ctx context.Context, id string, children bool) ([]string, error) {
	column := "def_block_id"
	if children {
		column = "def_block_root_id"
	}
	return q.ids(ctx, "SELECT DISTINCT block_id FROM refs WHERE "+column+"=?", id)
}
func (q References) ByPair(ctx context.Context, def, ref string) ([]RefRecord, error) {
	return q.queryRefs(ctx, "SELECT * FROM refs WHERE def_block_id=? AND block_id=?", def, ref)
}

// Original recursive children are FTS/block logical parents, not physical tree
// parents. Detect corrupt cycles instead of recursing forever. Rows are closed
// before following children, including with one-connection databases.
func (q References) BlockChildren(ctx context.Context, id string) (ret []string, err error) {
	stack := []string{id}
	seen := map[string]bool{}
	for len(stack) > 0 {
		id = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[id] {
			return nil, errors.New("cyclic_block_parent")
		}
		seen[id] = true
		if len(seen) > 100000 {
			return nil, errors.New("reference_query_budget")
		}
		ret = append(ret, id)
		children, e := q.ids(ctx, "SELECT id FROM blocks WHERE parent_id=?", id)
		if e != nil {
			return nil, e
		}
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
	}
	return ret, ctx.Err()
}
func (q References) ByDefinition(ctx context.Context, id string, children bool) ([]RefRecord, error) {
	if !children {
		return q.queryRefs(ctx, "SELECT * FROM refs WHERE def_block_id=?", id)
	}
	ids, err := q.BlockChildren(ctx, id)
	if err != nil {
		return nil, err
	}
	var ret []RefRecord
	for start := 0; start < len(ids); start += 512 {
		end := min(start+512, len(ids))
		args := make([]interface{}, end-start)
		for i, id := range ids[start:end] {
			args[i] = id
		}
		rows, e := q.queryRefs(ctx, "SELECT * FROM refs WHERE def_block_id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args...)
		if e != nil {
			return nil, e
		}
		ret = append(ret, rows...)
	}
	return ret, nil
}

func scanBlock(row interface{ Scan(...interface{}) error }) (b BlockRecord, err error) {
	err = row.Scan(blockFields(&b)...)
	return
}
func blockFields(b *BlockRecord) []interface{} {
	return []interface{}{&b.ID, &b.ParentID, &b.RootID, &b.Hash, &b.Box, &b.Path, &b.HPath, &b.Name, &b.Alias, &b.Memo, &b.Tag, &b.Content, &b.FContent, &b.Markdown, &b.Length, &b.Type, &b.SubType, &b.IAL, &b.Sort, &b.Created, &b.Updated}
}
func (q References) DefinitionRoots(ctx context.Context, refRoot string) (ret []BlockRecord, err error) {
	rows, err := Query(ctx, q.DB, "SELECT * FROM blocks WHERE id IN (SELECT DISTINCT def_block_root_id FROM refs WHERE root_id=?)", refRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		b, e := scanBlock(rows)
		if e != nil {
			return nil, e
		}
		ret = append(ret, b)
	}
	return ret, rows.Err()
}

func (q References) ReferringRoots(ctx context.Context, ids []string) (map[string][]BlockRecord, error) {
	ret := map[string][]BlockRecord{}
	seen := map[string]bool{}
	var unique []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	for start := 0; start < len(unique); start += 512 {
		end := min(start+512, len(unique))
		args := make([]interface{}, end-start)
		for i, id := range unique[start:end] {
			args[i] = id
		}
		err := func() error {
			rows, e := Query(ctx, q.DB, "SELECT b.*,r.def_block_root_id FROM refs AS r,blocks AS b ON r.def_block_root_id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+") AND b.id=r.root_id", args...)
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var b BlockRecord
				var root string
				fields := append(blockFields(&b), &root)
				if e = rows.Scan(fields...); e != nil {
					return e
				}
				ret[root] = append(ret[root], b)
			}
			return rows.Err()
		}()
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}

// Fragments are existing trusted internal search/indexignore SQL policy, not
// an endpoint accepting arbitrary user SQL. Keep GROUP/LIMIT placement intact.
func (q References) Recent(ctx context.Context, onlyDoc bool, typeFilter string, ignore []string) ([]RefRecord, error) {
	stmt := "SELECT r.* FROM refs AS r,blocks AS b WHERE b.id=r.def_block_id AND b.type IN " + typeFilter
	if onlyDoc {
		stmt = "SELECT r.* FROM refs AS r,blocks AS b WHERE b.id=r.def_block_id AND b.type='d'"
	}
	for _, line := range ignore {
		stmt += " AND " + line
	}
	stmt += " GROUP BY r.def_block_id ORDER BY r.id DESC LIMIT 32"
	if err := ValidateReadSQL(stmt); err != nil {
		return nil, err
	}
	return q.queryRefs(ctx, stmt)
}

type DefinitionReference struct{ Definition, Reference BlockRecord }

func (q References) ByAnnotation(ctx context.Context, id string) ([]string, error) {
	out, e := q.ids(ctx, "SELECT block_id FROM file_annotation_refs WHERE annotation_id = ?", id)
	if out == nil && e == nil {
		out = []string{}
	}
	return out, e
}

func (q References) DefinitionPairs(ctx context.Context, condition string, limit int) (ret []DefinitionReference, err error) {
	stmt := "SELECT ref.*,r.block_id||'@'||r.def_block_id FROM blocks AS ref,refs AS r WHERE ref.id=r.block_id"
	if condition != "" {
		stmt += " AND " + condition
	}
	refs := map[string]BlockRecord{}
	err = func() error {
		rows, e := Query(ctx, q.DB, stmt)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var b BlockRecord
			var rel string
			if e = rows.Scan(append(blockFields(&b), &rel)...); e != nil {
				return e
			}
			refs[rel] = b
		}
		return rows.Err()
	}()
	if err != nil {
		return nil, err
	}
	defs := map[string]BlockRecord{}
	err = func() error {
		rows, e := Query(ctx, q.DB, "SELECT def.* FROM blocks AS def,refs AS r WHERE def.id=r.def_block_id LIMIT ?", limit)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			b, e := scanBlock(rows)
			if e != nil {
				return e
			}
			defs[b.ID] = b
		}
		return rows.Err()
	}()
	if err != nil {
		return nil, err
	}
	for rel, ref := range refs {
		parts := strings.Split(rel, "@")
		if len(parts) < 2 {
			continue
		}
		if def, ok := defs[parts[1]]; ok {
			ret = append(ret, DefinitionReference{Definition: def, Reference: ref})
		}
	}
	return ret, nil
}

func (q References) DefinitionIDsByText(ctx context.Context, keyword string, exclude []string, caseSensitive bool) (ret []string, err error) {
	excluded := map[string]bool{}
	for _, id := range exclude {
		excluded[id] = true
	}
	seen := map[string]bool{}
	add := func(id string) {
		if !excluded[id] && !seen[id] {
			seen[id] = true
			ret = append(ret, id)
		}
	}
	cmp := "LIKE"
	if caseSensitive {
		cmp = "="
	}
	ids, err := q.ids(ctx, "SELECT DISTINCT def_block_id FROM refs WHERE content "+cmp+" ?", keyword)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		add(id)
	}
	err = func() error {
		rows, e := Query(ctx, q.DB, "SELECT DISTINCT id,name,alias FROM blocks WHERE name=? OR alias LIKE ?", keyword, "%"+keyword+"%")
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var id, name, alias string
			if e = rows.Scan(&id, &name, &alias); e != nil {
				return e
			}
			if name == keyword {
				add(id)
				continue
			}
			hit := false
			for _, a := range strings.Split(alias, ",") {
				if a != "" && a == keyword {
					hit = true
				}
			}
			if strings.Contains(alias, keyword) && !hit {
				continue
			}
			add(id)
		}
		return rows.Err()
	}()
	if err != nil {
		return nil, err
	}
	ids, err = q.ids(ctx, "SELECT id FROM blocks WHERE type='d' AND content "+cmp+" ?", keyword)
	if err != nil {
		return nil, err
	}
	count := 0
	for _, id := range ids {
		if excluded[id] {
			continue
		}
		add(id)
		count++
		if count >= 32 {
			break
		}
	}
	return ret, nil
}

func (q References) VirtualKeywords(ctx context.Context, name, alias, anchor, doc bool, ignore, refIgnore []string) (ret []string, err error) {
	seen := map[string]bool{}
	read := func(stmt string, clauses []string, split bool, limit bool) error {
		for _, line := range clauses {
			stmt += " AND " + line
		}
		if limit {
			stmt += " LIMIT 10240"
		}
		values, e := q.ids(ctx, stmt)
		if e != nil {
			return e
		}
		for _, v := range values {
			parts := []string{v}
			if split {
				parts = strings.Split(v, ",")
			}
			for _, p := range parts {
				if strings.TrimSpace(p) != "" && !seen[p] {
					seen[p] = true
					ret = append(ret, p)
				}
			}
		}
		return nil
	}
	if name {
		if err = read("SELECT name FROM blocks WHERE name!=''", ignore, true, true); err != nil {
			return nil, err
		}
	}
	if alias {
		if err = read("SELECT alias FROM blocks WHERE alias!=''", ignore, true, true); err != nil {
			return nil, err
		}
	}
	if anchor {
		if err = read("SELECT DISTINCT content FROM refs WHERE 1=1", refIgnore, false, true); err != nil {
			return nil, err
		}
	}
	if doc {
		if err = read("SELECT content FROM blocks WHERE type='d'", ignore, true, false); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(ret, func(i, j int) bool { return len(ret[i]) >= len(ret[j]) })
	return ret, nil
}
