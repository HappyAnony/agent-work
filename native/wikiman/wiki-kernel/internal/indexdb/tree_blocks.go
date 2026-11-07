// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Adapted from sql/upsert.go and sql/database.go; explicit transaction ownership.
package indexdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
)

// BlockBatch is a derived-index notification, not a source commit receipt.
// Callers must defer external notifications until their transaction commits.
type BlockBatch struct {
	Count int
	Hash  string
}

// InsertBlocks preserves all 21 original columns, 512-row batches and the
// case-sensitive FTS policy. It never chooses a database or touches a cache.
// A failure leaves rollback to the transaction owner, including earlier batches.
func InsertBlocks(ctx context.Context, tx *sql.Tx, blocks []BlockRecord, caseSensitive bool, inserted func(BlockBatch)) error {
	if tx == nil {
		return ErrNoDatabase
	}
	const columns = "(id,parent_id,root_id,hash,box,path,hpath,name,alias,memo,tag,content,fcontent,markdown,length,type,subtype,ial,sort,created,updated)"
	const placeholder = "(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
	for start := 0; start < len(blocks); start += 512 {
		bulk := blocks[start:min(start+512, len(blocks))]
		args := make([]interface{}, 0, len(bulk)*21)
		var hashes strings.Builder
		for _, b := range bulk {
			args = append(args, b.ID, b.ParentID, b.RootID, b.Hash, b.Box, b.Path, b.HPath, b.Name, b.Alias, b.Memo, b.Tag, b.Content, b.FContent, b.Markdown, b.Length, b.Type, b.SubType, b.IAL, b.Sort, b.Created, b.Updated)
			hashes.WriteString(b.Hash)
		}
		values := strings.TrimSuffix(strings.Repeat(placeholder+",", len(bulk)), ",")
		for _, table := range mainBlockTables(caseSensitive) {
			if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+columns+" VALUES "+values, args...); err != nil {
				return err
			}
		}
		if inserted != nil {
			hashes.WriteString("blocksfts")
			inserted(BlockBatch{Count: len(bulk), Hash: fmt.Sprintf("%x", sha256.Sum256([]byte(hashes.String())))[:7]})
		}
	}
	return ctx.Err()
}

// SelectUpsertBlocks retains the original hash-based diff: unchanged blocks
// keep their physical ROWID; changed/new blocks are inserted in input order.
// The read uses the writing transaction, not a global DB or an admission-time
// snapshot. Read/scan/cancellation errors must abort the owning transaction.
func SelectUpsertBlocks(ctx context.Context, tx *sql.Tx, rootID string, blocks []BlockRecord) (changed []BlockRecord, remove []string, err error) {
	if tx == nil {
		return nil, nil, ErrNoDatabase
	}
	rows, err := tx.QueryContext(ctx, "SELECT id, hash FROM blocks WHERE root_id = ?", rootID)
	if err != nil {
		return nil, nil, err
	}
	old := map[string]string{}
	for rows.Next() {
		var id, hash string
		if err = rows.Scan(&id, &hash); err != nil {
			rows.Close()
			return nil, nil, err
		}
		old[id] = hash
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if closeErr != nil {
		return nil, nil, closeErr
	}
	newHashes := map[string]string{}
	for _, b := range blocks {
		newHashes[b.ID] = b.Hash
	}
	unchanged := map[string]bool{}
	for id, hash := range old {
		if newHash, ok := newHashes[id]; ok {
			unchanged[id] = hash == newHash
		} else {
			remove = append(remove, id)
		}
	}
	for _, b := range blocks {
		if !unchanged[b.ID] {
			changed = append(changed, b)
			remove = append(remove, b.ID)
		}
	}
	return changed, remove, ctx.Err()
}

// DeleteIndexedBlocksByIDs uses the original physical ROWID mapping across
// blocks and its FTS mirrors, not an FTS-id deletion with different semantics.
// Bound parameters and chunks also handle quoted IDs and large documents.
func DeleteIndexedBlocksByIDs(ctx context.Context, tx *sql.Tx, ids []string, caseSensitive bool) error {
	if tx == nil {
		return ErrNoDatabase
	}
	for start := 0; start < len(ids); start += 512 {
		bulk := ids[start:min(start+512, len(ids))]
		args := make([]interface{}, len(bulk))
		for i, id := range bulk {
			args[i] = id
		}
		marks := strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")
		rows, err := tx.QueryContext(ctx, "SELECT ROWID FROM blocks WHERE id IN ("+marks+")", args...)
		if err != nil {
			return err
		}
		var rowIDs []interface{}
		for rows.Next() {
			var rowID int64
			if err = rows.Scan(&rowID); err != nil {
				rows.Close()
				return err
			}
			rowIDs = append(rowIDs, rowID)
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		// A damaged legacy index may contain multiple physical rows per ID.
		for offset := 0; offset < len(rowIDs); offset += 512 {
			part := rowIDs[offset:min(offset+512, len(rowIDs))]
			where := " WHERE ROWID IN (" + strings.TrimSuffix(strings.Repeat("?,", len(part)), ",") + ")"
			for _, table := range mainBlockTables(caseSensitive) {
				if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+where, part...); err != nil {
					return err
				}
			}
		}
	}
	return ctx.Err()
}
