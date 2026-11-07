// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original treenode/blocktree.go queries and physical-parent projection.
package indexdb

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"wiki-kernel/internal/nodetype"
)

type BlockTreeRecord struct{ ID, RootID, ParentID, BoxID, Path, HPath, Updated, Type string }
type BlockTreeFilter struct {
	Kind, Value, BoxID, ParentID string
	IDs                          []string
}
type BlockTrees struct{ DB *sql.DB }

func blockTreeWhere(f BlockTreeFilter) (string, []interface{}, error) {
	switch f.Kind {
	case "all":
		return "", nil, nil
	case "id":
		return " WHERE id = ?", []interface{}{f.Value}, nil
	case "root":
		return " WHERE root_id = ?", []interface{}{f.Value}, nil
	case "box":
		return " WHERE box_id = ?", []interface{}{f.Value}, nil
	case "path":
		return " WHERE path = ?", []interface{}{f.Value}, nil
	case "root-path":
		return " WHERE box_id = ? AND path = ? AND type = 'd'", []interface{}{f.BoxID, f.Value}, nil
	case "box-path":
		return " WHERE box_id = ? AND path = ?", []interface{}{f.BoxID, f.Value}, nil
	case "root-hpath":
		return " WHERE box_id = ? AND hpath = ? AND type = 'd'", []interface{}{f.BoxID, gulu.Str.RemoveInvisible(f.Value)}, nil
	case "preferred":
		return " WHERE box_id = ? AND hpath = ? AND parent_id = ? LIMIT 1", []interface{}{f.BoxID, gulu.Str.RemoveInvisible(f.Value), f.ParentID}, nil
	case "prefix":
		return " WHERE path LIKE ?", []interface{}{f.Value + "%"}, nil
	case "type":
		return " WHERE type = ?", []interface{}{f.Value}, nil
	}
	return "", nil, fmt.Errorf("invalid_blocktree_filter")
}
func (q BlockTrees) Query(ctx context.Context, f BlockTreeFilter) (ret []BlockTreeRecord, err error) {
	if q.DB == nil {
		return nil, ErrNoDatabase
	}
	if f.Kind == "ids" {
		for start := 0; start < len(f.IDs); start += 512 {
			end := min(start+512, len(f.IDs))
			args := make([]interface{}, end-start)
			for i, id := range f.IDs[start:end] {
				args[i] = id
			}
			part, e := q.query(ctx, " WHERE id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args)
			if e != nil {
				return nil, e
			}
			ret = append(ret, part...)
		}
		return ret, nil
	}
	where, args, err := blockTreeWhere(f)
	if err != nil {
		return nil, err
	}
	return q.query(ctx, where, args)
}
func (q BlockTrees) query(ctx context.Context, where string, args []interface{}) (ret []BlockTreeRecord, err error) {
	rows, err := q.DB.QueryContext(ctx, "SELECT * FROM blocktrees"+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var b BlockTreeRecord
		if err = rows.Scan(&b.ID, &b.RootID, &b.ParentID, &b.BoxID, &b.Path, &b.HPath, &b.Updated, &b.Type); err != nil {
			return nil, err
		}
		ret = append(ret, b)
	}
	return ret, rows.Err()
}
func (q BlockTrees) Count(ctx context.Context, f BlockTreeFilter) (n int, err error) {
	if q.DB == nil {
		return 0, ErrNoDatabase
	}
	where, args, err := blockTreeWhere(f)
	if err != nil {
		return 0, err
	}
	err = q.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM blocktrees"+where, args...).Scan(&n)
	return
}
func (q BlockTrees) Remove(ctx context.Context, f BlockTreeFilter) (removed []string, err error) {
	if q.DB == nil {
		return nil, ErrNoDatabase
	}
	if f.Kind != "id" && f.Kind != "root" && f.Kind != "box" && f.Kind != "prefix" && f.Kind != "box-path" {
		return nil, ErrMainMutation
	}
	where, args, err := blockTreeWhere(f)
	if err != nil {
		return nil, err
	}
	err = Transaction(ctx, q.DB, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, "SELECT id FROM blocktrees"+where, args...)
		if e != nil {
			return e
		}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			removed = append(removed, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, "DELETE FROM blocktrees"+where, args...)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

func (q BlockTrees) PathDifference(ctx context.Context, box string, paths []string) (redundant, missing []string, err error) {
	rows, err := q.Query(ctx, BlockTreeFilter{Kind: "box", Value: box})
	if err != nil {
		return nil, nil, err
	}
	known, wanted := map[string]bool{}, map[string]bool{}
	for _, b := range rows {
		known[b.Path] = true
	}
	for _, p := range paths {
		wanted[p] = true
	}
	for p := range known {
		if !wanted[p] {
			redundant = append(redundant, p)
		}
	}
	for p := range wanted {
		if !known[p] {
			missing = append(missing, p)
		}
	}
	return
}
func (q BlockTrees) RootUpdated(ctx context.Context) (ret map[string]string, err error) {
	ret = map[string]string{}
	rows, err := q.Query(ctx, BlockTreeFilter{Kind: "type", Value: "d"})
	if err != nil {
		return nil, err
	}
	for _, b := range rows {
		if b.ID == b.RootID {
			ret[b.RootID] = b.Updated
		}
	}
	return
}
func ProjectBlockTree(tree *parse.Tree) []BlockTreeRecord {
	var ret []BlockTreeRecord
	ast.Walk(tree.Root, func(n *ast.Node, e bool) ast.WalkStatus {
		if e && n.IsBlock() && n.ID != "" {
			ret = append(ret, blockTreeRow(n, tree))
		}
		return ast.WalkContinue
	})
	return ret
}
func blockTreeRow(n *ast.Node, t *parse.Tree) BlockTreeRecord {
	return BlockTreeRecord{ID: n.ID, RootID: t.ID, ParentID: physicalParent(n), BoxID: t.Box, Path: t.Path, HPath: t.HPath, Updated: n.IALAttr("updated"), Type: nodetype.TypeAbbr(n.Type.String())}
}
func insertBlockTrees(ctx context.Context, tx *sql.Tx, rows []BlockTreeRecord) error {
	for _, b := range rows {
		if _, err := tx.ExecContext(ctx, "INSERT INTO blocktrees(id,root_id,parent_id,box_id,path,hpath,updated,type) VALUES(?,?,?,?,?,?,?,?)", b.ID, b.RootID, b.ParentID, b.BoxID, b.Path, b.HPath, b.Updated, b.Type); err != nil {
			return err
		}
	}
	return ctx.Err()
}
func (q BlockTrees) Index(ctx context.Context, t *parse.Tree) error {
	return Transaction(ctx, q.DB, func(tx *sql.Tx) error { return insertBlockTrees(ctx, tx, ProjectBlockTree(t)) })
}

// Upsert intentionally preserves original changed-container descendant expansion
// (including repeated rows) and its explicit separate deletion contract.
func (q BlockTrees) Upsert(ctx context.Context, t *parse.Tree) error {
	return Transaction(ctx, q.DB, func(tx *sql.Tx) error {
		old := map[string]BlockTreeRecord{}
		rows, e := tx.QueryContext(ctx, "SELECT * FROM blocktrees WHERE root_id=?", t.ID)
		if e != nil {
			return e
		}
		for rows.Next() {
			var b BlockTreeRecord
			if e = rows.Scan(&b.ID, &b.RootID, &b.ParentID, &b.BoxID, &b.Path, &b.HPath, &b.Updated, &b.Type); e != nil {
				rows.Close()
				return e
			}
			old[b.ID] = b
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		var changed []BlockTreeRecord
		ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if !enter || !n.IsBlock() || n.ID == "" {
				return ast.WalkContinue
			}
			now := blockTreeRow(n, t)
			prev, found := old[n.ID]
			if !found || prev.Updated != now.Updated || prev.Type != now.Type || prev.Path != now.Path || prev.BoxID != now.BoxID || prev.HPath != now.HPath {
				if !n.IsContainerBlock() || n.Type == ast.NodeDocument {
					changed = append(changed, now)
				} else {
					ast.Walk(n, func(child *ast.Node, in bool) ast.WalkStatus {
						if in && child.IsBlock() {
							changed = append(changed, blockTreeRow(child, t))
						}
						return ast.WalkContinue
					})
				}
			}
			return ast.WalkContinue
		})
		for start := 0; start < len(changed); start += 512 {
			part := changed[start:min(start+512, len(changed))]
			args := make([]interface{}, len(part))
			for i, b := range part {
				args[i] = b.ID
			}
			if _, e = tx.ExecContext(ctx, "DELETE FROM blocktrees WHERE id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args...); e != nil {
				return e
			}
		}
		return insertBlockTrees(ctx, tx, changed)
	})
}

// Replace is an explicit source-snapshot refresh, not the original incremental
// Upsert. It atomically removes stale/deleted rows before publishing the snapshot.
func (q BlockTrees) Replace(ctx context.Context, root string, rows []BlockTreeRecord) error {
	return Transaction(ctx, q.DB, func(tx *sql.Tx) error {
		stmt := "DELETE FROM blocktrees"
		var args []interface{}
		if root != "" {
			stmt += " WHERE root_id=?"
			args = append(args, root)
		}
		if _, e := tx.ExecContext(ctx, stmt, args...); e != nil {
			return e
		}
		return insertBlockTrees(ctx, tx, rows)
	})
}
