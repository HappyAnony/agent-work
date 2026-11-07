//go:build fts5

package indexdb

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestOriginalAssetQueueRollbackCopiesDeleteAndIsolation(t *testing.T) {
	ctx := context.Background()
	a, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	db, _ := a.Database(Assets)
	other, _ := b.Database(Assets)
	var q AssetQueue
	items := make([]AssetContentEntry, 513)
	for i := range items {
		items[i] = AssetContentEntry{ID: "same-id", Name: "original", Path: "assets/quote'中文.pdf", Size: 9, Updated: 9007199254740993, Content: "synthetic"}
	}
	if err = q.Index(items); err != nil {
		t.Fatal(err)
	}
	items[0].Name = "mutated"
	cancelled, cancel := context.WithCancel(ctx)
	if err = q.Flush(cancelled, db, func(AssetProgress) { cancel() }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM asset_contents_fts_case_insensitive").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = q.Flush(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM asset_contents_fts_case_insensitive WHERE name='original' AND updated=9007199254740993").Scan(&count); err != nil || count != 513 {
		t.Fatal(count, err)
	}
	if err = other.QueryRow("SELECT COUNT(*) FROM asset_contents_fts_case_insensitive").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = q.DeletePath("assets/quote'中文.pdf"); err != nil {
		t.Fatal(err)
	}
	if err = q.Close(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
	if err = q.DeletePath("late"); !errors.Is(err, ErrAssetQueueClosed) {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM asset_contents_fts_case_insensitive").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestOriginalMainContentQueueCoalescesAndUsesAllFTS(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	block := BlockRecord{ID: "same-id", Type: "query_embed", Markdown: "{{SELECT * FROM blocks}}"}
	value := reflect.ValueOf(block)
	args := make([]interface{}, value.NumField())
	for i := range args {
		args[i] = value.Field(i).Interface()
	}
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		if _, err = db.Exec("INSERT INTO "+table+" VALUES("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args...); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := EmptyEmbedBlocks(ctx, db)
	if err != nil || len(rows) != 1 || rows[0].Markdown != block.Markdown {
		t.Fatal(rows, err)
	}
	if _, err = db.Exec("CREATE TABLE audit(content);CREATE TRIGGER audit_update AFTER UPDATE ON blocks BEGIN INSERT INTO audit VALUES(new.content); END;"); err != nil {
		t.Fatal(err)
	}
	var q ContentQueue
	if err = q.Update("same-id", "first"); err != nil {
		t.Fatal(err)
	}
	if err = q.Update("same-id", "last"); err != nil {
		t.Fatal(err)
	}
	if err = q.Flush(ctx, nil, false); !errors.Is(err, ErrNoDatabase) {
		t.Fatal(err)
	}
	if err = q.Close(ctx, db, false); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		var text string
		if err = db.QueryRow("SELECT content FROM " + table + " WHERE id='same-id'").Scan(&text); err != nil || text != "last" {
			t.Fatal(table, text, err)
		}
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM audit").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err = q.Update("late", "value"); !errors.Is(err, ErrContentQueueClosed) {
		t.Fatal(err)
	}
}

func TestContentQueueRestoredAndNewerWorkKeepLatestValue(t *testing.T) {
	var q ContentQueue
	if err := q.Update("A", "admitted during flush"); err != nil {
		t.Fatal(err)
	}
	q.restore([]contentUpdate{{id: "A", content: "old failed"}, {id: "B", content: "other"}})
	if err := q.Update("A", "newest"); err != nil {
		t.Fatal(err)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) != 2 || q.pending[0].content != "newest" || q.pending[1].id != "B" {
		t.Fatal(q.pending)
	}
}
