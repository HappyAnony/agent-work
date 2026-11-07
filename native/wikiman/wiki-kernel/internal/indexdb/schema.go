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
	"fmt"
)

type Kind uint8

const (
	Main Kind = iota
	History
	Assets
	BlockTree
)
const SchemaVersion = "20220501"

// Statements is the original schema shared by compat and Context lifecycles.
// Each call returns a fresh slice, not mutable process-wide table definitions.
func Statements(kind Kind) ([]string, error) {
	switch kind {
	case Main:
		return []string{
			"DROP TABLE IF EXISTS stat",
			"CREATE TABLE stat (key, value)",
			"DROP TABLE IF EXISTS blocks",
			"CREATE TABLE blocks (id, parent_id, root_id, hash, box, path, hpath, name, alias, memo, tag, content, fcontent, markdown, length, type, subtype, ial, sort, created, updated)",
			"CREATE INDEX idx_blocks_id ON blocks(id)",
			"CREATE INDEX idx_blocks_parent_id ON blocks(parent_id)",
			"CREATE INDEX idx_blocks_root_id ON blocks(root_id)",
			"DROP TABLE IF EXISTS blocks_fts",
			"CREATE VIRTUAL TABLE blocks_fts USING fts5(id UNINDEXED, parent_id UNINDEXED, root_id UNINDEXED, hash UNINDEXED, box UNINDEXED, path UNINDEXED, hpath, name, alias, memo, tag, content, fcontent, markdown UNINDEXED, length UNINDEXED, type UNINDEXED, subtype UNINDEXED, ial, sort UNINDEXED, created UNINDEXED, updated UNINDEXED, tokenize=\"siyuan\")",
			"DROP TABLE IF EXISTS blocks_fts_case_insensitive",
			"CREATE VIRTUAL TABLE blocks_fts_case_insensitive USING fts5(id UNINDEXED, parent_id UNINDEXED, root_id UNINDEXED, hash UNINDEXED, box UNINDEXED, path UNINDEXED, hpath, name, alias, memo, tag, content, fcontent, markdown UNINDEXED, length UNINDEXED, type UNINDEXED, subtype UNINDEXED, ial, sort UNINDEXED, created UNINDEXED, updated UNINDEXED, tokenize=\"siyuan case_insensitive\")",
			"DROP TABLE IF EXISTS spans",
			"CREATE TABLE spans (id, block_id, root_id, box, path, content, markdown, type, ial)",
			"CREATE INDEX idx_spans_root_id ON spans(root_id)",
			"DROP TABLE IF EXISTS assets",
			"CREATE TABLE assets (id, block_id, root_id, box, docpath, path, name, title, hash)",
			"CREATE INDEX idx_assets_root_id ON assets(root_id)",
			"DROP TABLE IF EXISTS attributes",
			"CREATE TABLE attributes (id, name, value, type, block_id, root_id, box, path)",
			"CREATE INDEX idx_attributes_block_id ON attributes(block_id)",
			"CREATE INDEX idx_attributes_root_id ON attributes(root_id)",
			"DROP TABLE IF EXISTS refs",
			"CREATE TABLE refs (id, def_block_id, def_block_parent_id, def_block_root_id, def_block_path, block_id, root_id, box, path, content, markdown, type)",
			"DROP TABLE IF EXISTS file_annotation_refs",
			"CREATE TABLE file_annotation_refs (id, file_path, annotation_id, block_id, root_id, box, path, content, type)",
		}, nil
	case History:
		return []string{
			"DROP TABLE IF EXISTS histories_fts_case_insensitive",
			"CREATE VIRTUAL TABLE histories_fts_case_insensitive USING fts5(id UNINDEXED, type UNINDEXED, op UNINDEXED, title, content, path UNINDEXED, created UNINDEXED, tokenize=\"siyuan case_insensitive\")",
		}, nil
	case Assets:
		return []string{
			"DROP TABLE IF EXISTS asset_contents_fts_case_insensitive",
			"CREATE VIRTUAL TABLE asset_contents_fts_case_insensitive USING fts5(id UNINDEXED, name, ext, path, size UNINDEXED, updated UNINDEXED, content, tokenize=\"siyuan case_insensitive\")",
		}, nil
	case BlockTree:
		return []string{
			"DROP TABLE IF EXISTS blocktrees",
			"CREATE TABLE blocktrees (id, root_id, parent_id, box_id, path, hpath, updated, type)",
			"CREATE INDEX idx_blocktrees_id ON blocktrees(id)",
		}, nil
	default:
		return nil, fmt.Errorf("invalid index kind: %d", kind)
	}
}

// Reset rebuilds derived tables atomically. It does not select global paths,
// delete source files, launch workers, or terminate the caller on SQL errors.
func Reset(ctx context.Context, db *sql.DB, kind Kind, version string) error {
	statements, err := Statements(kind)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range statements {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initialize index %d: %w", kind, err)
		}
	}
	if kind == Main {
		if _, err = tx.ExecContext(ctx, "INSERT INTO stat VALUES (?,?)", "siyuan_database_ver", version); err != nil {
			return err
		}
	}
	return tx.Commit()
}
