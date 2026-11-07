// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original sql/database.go, block.go, block_ref.go and asset.go algorithms.
package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"path"
	"strings"
)

// MainMutation is an owned value, not a mutable parse.Tree or source write.
// Reference/content projection is performed from a detached source tree before
// admission, including full original tree projections.
type MainMutation struct {
	Action, ID, Box, Path, HPath, IAL, Updated string
	Content                                    string
	Tree                                       *TreeRecords
	Refs                                       []RefRecord
	Annotations                                []AnnotationRecord
	IDs, Hashes                                []string
}

var ErrMainMutation = errors.New("main_mutation_not_contextualized")

func (m MainMutation) Validate() error {
	switch m.Action {
	case "index", "upsert":
		if m.Tree == nil || m.ID != m.Tree.RootID || !m.Tree.Valid() {
			return ErrMainMutation
		}
		return nil
	case "delete", "delete_id", "delete_ids", "rename", "rename_sub_tree", "delete_box", "delete_box_refs", "delete_refs", "delete_assets", "update_refs", "index_node":
		return nil
	}
	return ErrMainMutation
}

func (m MainMutation) key() (key struct{ action, id, box, path string }, coalesce bool) {
	key.action = m.Action
	switch m.Action {
	case "delete_ids", "delete_assets":
		return key, false
	case "delete":
		key.box, key.path = m.Box, m.Path
	case "delete_box", "delete_box_refs":
		key.box = m.Box
	default:
		key.id = m.ID
	}
	return key, true
}

func mainBlockTables(caseSensitive bool) []string {
	ret := []string{"blocks", "blocks_fts"}
	if !caseSensitive {
		ret = append(ret, "blocks_fts_case_insensitive")
	}
	return ret
}

// ApplyMainMutation shares the original table coverage, path LIKE semantics,
// case-sensitive FTS rule, rename IAL and root title/fcontent/updated behavior.
// Values are parameters (including quoted IDs/hashes); table names are fixed.
// Cache invalidation and event routing are the caller's explicit ownership.
func ApplyMainMutation(ctx context.Context, tx *sql.Tx, m MainMutation, caseSensitive bool) error {
	if err := m.Validate(); err != nil {
		return err
	}
	exec := func(stmt string, args ...interface{}) error { _, err := tx.ExecContext(ctx, stmt, args...); return err }
	tables := mainBlockTables(caseSensitive)
	all := append(append([]string{}, tables...), "spans", "assets", "refs", "file_annotation_refs", "attributes")
	switch m.Action {
	case "index", "upsert":
		_, err := WriteTree(ctx, tx, *m.Tree, m.Action == "upsert", caseSensitive)
		return err
	case "update_refs":
		return ReplaceReferences(ctx, tx, m.Box, m.Path, m.Refs, m.Annotations)
	case "index_node":
		return UpdateBlockContent(ctx, tx, m.ID, m.Content, caseSensitive)
	case "delete_id", "delete_ids":
		ids := m.IDs
		if m.Action == "delete_id" {
			ids = []string{m.ID}
		}
		// Stay below SQLite's parameter ceiling without changing transaction atomicity.
		for start := 0; start < len(ids); start += 512 {
			end := min(start+512, len(ids))
			args := make([]interface{}, end-start)
			for i, id := range ids[start:end] {
				args[i] = id
			}
			where := " WHERE root_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(args)), ",") + ")"
			for _, table := range all {
				if err := exec("DELETE FROM "+table+where, args...); err != nil {
					return err
				}
			}
		}
	case "delete":
		for _, table := range all {
			column := "path"
			if table == "assets" {
				column = "docpath"
			}
			if err := exec("DELETE FROM "+table+" WHERE box = ? AND "+column+" LIKE ?", m.Box, m.Path+"%"); err != nil {
				return err
			}
		}
	case "delete_box", "delete_box_refs", "delete_refs":
		selected := all
		if m.Action != "delete_box" {
			selected = []string{"refs", "file_annotation_refs"}
		}
		for _, table := range selected {
			where := " WHERE box = ?"
			args := []interface{}{m.Box}
			if m.Action == "delete_refs" {
				where += " AND path = ?"
				args = append(args, m.Path)
			}
			if err := exec("DELETE FROM "+table+where, args...); err != nil {
				return err
			}
		}
	case "delete_assets":
		// Original behavior also removes empty hashes, but an empty queue input is a no-op.
		if len(m.Hashes) == 0 {
			return nil
		}
		if err := exec("DELETE FROM assets WHERE hash = ''"); err != nil {
			return err
		}
		for start := 0; start < len(m.Hashes); start += 512 {
			end := min(start+512, len(m.Hashes))
			args := make([]interface{}, end-start)
			for i, h := range m.Hashes[start:end] {
				args[i] = h
			}
			if err := exec("DELETE FROM assets WHERE hash IN ("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args...); err != nil {
				return err
			}
		}
	case "rename", "rename_sub_tree":
		if err := UpdateBlockPaths(ctx, tx, m, caseSensitive); err != nil {
			return err
		}
		if m.Action == "rename" {
			return UpdateRootContent(ctx, tx, path.Base(m.HPath), m.Updated, m.ID, caseSensitive)
		}
	}
	return nil
}

func UpdateBlockPaths(ctx context.Context, tx *sql.Tx, m MainMutation, caseSensitive bool) error {
	if m.Action != "rename" && m.Action != "rename_sub_tree" {
		return ErrMainMutation
	}
	exec := func(stmt string, args ...interface{}) error { _, err := tx.ExecContext(ctx, stmt, args...); return err }
	for _, table := range mainBlockTables(caseSensitive) {
		stmt := "UPDATE " + table + " SET hpath = ?, ial = ? WHERE root_id = ?"
		args := []interface{}{m.HPath, m.IAL, m.ID}
		if m.Action == "rename_sub_tree" {
			stmt = "UPDATE " + table + " SET box = ?, path = ?, hpath = ?, ial = ? WHERE root_id = ?"
			args = []interface{}{m.Box, m.Path, m.HPath, m.IAL, m.ID}
		}
		if err := exec(stmt, args...); err != nil {
			return err
		}
	}

	return nil
}

func UpdateRootContent(ctx context.Context, tx *sql.Tx, content, updated, id string, caseSensitive bool) error {
	for _, table := range mainBlockTables(caseSensitive) {
		if _, err := tx.ExecContext(ctx, "UPDATE "+table+" SET content = ?, fcontent = ?, updated = ? WHERE id = ?", content, content, updated, id); err != nil {
			return err
		}
	}
	return nil
}
