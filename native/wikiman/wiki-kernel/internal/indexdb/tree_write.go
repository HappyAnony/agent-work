// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Original sql/upsert.go table coverage and insertion order.
package indexdb

import (
	"context"
	"database/sql"
	ignore "github.com/sabhiram/go-gitignore"
	"path"
	"strings"
)

func (t TreeRecords) Clone() TreeRecords {
	t.Blocks = append([]BlockRecord(nil), t.Blocks...)
	t.Spans = append([]SpanRecord(nil), t.Spans...)
	t.Assets = append([]AssetRecord(nil), t.Assets...)
	t.Attributes = append([]AttributeRecord(nil), t.Attributes...)
	t.Refs = append([]RefRecord(nil), t.Refs...)
	t.Annotations = append([]AnnotationRecord(nil), t.Annotations...)
	t.IgnoreLines = append([]string(nil), t.IgnoreLines...)
	return t
}
func (t TreeRecords) Valid() bool {
	if t.RootID == "" || t.Box == "" || t.Path == "" {
		return false
	}
	for _, b := range t.Blocks {
		if b.ID == "" || b.RootID != t.RootID || b.Box != t.Box || b.Path != t.Path {
			return false
		}
	}
	for _, v := range t.Spans {
		if v.RootID != t.RootID || v.Box != t.Box || v.Path != t.Path {
			return false
		}
	}
	for _, v := range t.Assets {
		if v.RootID != t.RootID || v.Box != t.Box || v.DocPath != t.Path {
			return false
		}
	}
	for _, v := range t.Attributes {
		if v.RootID != t.RootID || v.Box != t.Box || v.Path != t.Path {
			return false
		}
	}
	for _, v := range t.Refs {
		if v.RootID != t.RootID || v.Box != t.Box || v.Path != t.Path {
			return false
		}
	}
	for _, v := range t.Annotations {
		if v.RootID != t.RootID || v.Box != t.Box || v.Path != t.Path {
			return false
		}
	}
	return true
}

// WriteTree runs inside its caller's transaction. Block batches are collected,
// not published, so rollback can never emit success notifications.
func WriteTree(ctx context.Context, tx *sql.Tx, t TreeRecords, upsert, caseSensitive bool) (batches []BlockBatch, err error) {
	if !t.Valid() {
		return nil, ErrMainMutation
	}
	blocks := t.Blocks
	if upsert {
		var remove []string
		blocks, remove, err = SelectUpsertBlocks(ctx, tx, t.RootID, blocks)
		if err != nil {
			return nil, err
		}
		if err = DeleteIndexedBlocksByIDs(ctx, tx, remove, caseSensitive); err != nil {
			return nil, err
		}
		for _, table := range []string{"spans", "assets", "attributes"} {
			if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE root_id=?", t.RootID); err != nil {
				return nil, err
			}
		}
		for _, table := range []string{"refs", "file_annotation_refs"} {
			if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE box=? AND path=?", t.Box, t.Path); err != nil {
				return nil, err
			}
		}
	}
	// Preserve the original position of indexignore: an ignored upsert still
	// removes changed/deleted blocks and auxiliary rows, retaining unchanged blocks.
	if len(t.IgnoreLines) > 0 && ignore.CompileIgnoreLines(t.IgnoreLines...).MatchesPath("/"+path.Join(t.Box, t.Path)) {
		return nil, nil
	}
	if err = InsertBlocks(ctx, tx, blocks, caseSensitive, func(b BlockBatch) { batches = append(batches, b) }); err != nil {
		return nil, err
	}
	if err = InsertBlockReferences(ctx, tx, t.Refs); err != nil {
		return nil, err
	}
	if err = InsertAnnotationReferences(ctx, tx, t.Annotations); err != nil {
		return nil, err
	}
	if len(t.Spans) > 0 {
		if _, err = tx.ExecContext(ctx, "DELETE FROM spans WHERE root_id=?", t.RootID); err != nil {
			return nil, err
		}
	}
	var rows [][]interface{}
	for _, v := range t.Spans {
		rows = append(rows, []interface{}{v.ID, v.BlockID, v.RootID, v.Box, v.Path, v.Content, v.Markdown, v.Type, v.IAL})
	}
	if err = insertTreeRows(ctx, tx, "spans", "id,block_id,root_id,box,path,content,markdown,type,ial", 9, rows); err != nil {
		return nil, err
	}
	rows = nil
	for _, v := range t.Assets {
		rows = append(rows, []interface{}{v.ID, v.BlockID, v.RootID, v.Box, v.DocPath, v.Path, v.Name, v.Title, v.Hash})
	}
	if err = insertTreeRows(ctx, tx, "assets", "id,block_id,root_id,box,docpath,path,name,title,hash", 9, rows); err != nil {
		return nil, err
	}
	rows = nil
	for _, v := range t.Attributes {
		rows = append(rows, []interface{}{v.ID, v.Name, v.Value, v.Type, v.BlockID, v.RootID, v.Box, v.Path})
	}
	err = insertTreeRows(ctx, tx, "attributes", "id,name,value,type,block_id,root_id,box,path", 8, rows)
	if err != nil {
		return nil, err
	}
	return batches, ctx.Err()
}
func insertTreeRows(ctx context.Context, tx *sql.Tx, table, columns string, width int, rows [][]interface{}) error {
	for start := 0; start < len(rows); start += 512 {
		bulk := rows[start:min(start+512, len(rows))]
		args := make([]interface{}, 0, len(bulk)*width)
		for _, row := range bulk {
			args = append(args, row...)
		}
		placeholder := "(" + strings.TrimSuffix(strings.Repeat("?,", width), ",") + ")"
		_, err := tx.ExecContext(ctx, "INSERT INTO "+table+"("+columns+") VALUES "+strings.TrimSuffix(strings.Repeat(placeholder+",", len(bulk)), ","), args...)
		if err != nil {
			return err
		}
	}
	return ctx.Err()
}
