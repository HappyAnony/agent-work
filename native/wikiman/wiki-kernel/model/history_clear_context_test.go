//go:build fts5

package model

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func TestHistoryClearActualOwnedQueueAndReadOnlyRejection(t *testing.T) {
	c, _, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	queue := sql.CaptureLegacyHistoryQueue()
	if e := queue.Index(ctx, []indexdb.HistoryEntry{{ID: "same", Path: "stamp/file", Created: "1"}}); e != nil {
		t.Fatal(e)
	}
	if e := queue.Flush(ctx, nil); e != nil {
		t.Fatal(e)
	}
	const box = "20261028140000-refbox1"
	dirs := []string{util.HistoryDir, filepath.Join(util.DataDir, box, ".siyuan", "history"), filepath.Join(util.DataDir, ".siyuan", "history"), filepath.Join(util.DataDir, "assets", ".siyuan", "history")}
	for _, dir := range dirs {
		if e := os.MkdirAll(filepath.Join(dir, "sub"), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "sub", "kept-until-clear"), []byte("history"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	c.persistenceReadOnly = true
	if e := ClearWorkspaceHistoryContext(ctx); e == nil {
		t.Fatal("readonly clear succeeded")
	}
	for _, dir := range dirs {
		if _, e := os.Stat(dir); e != nil {
			t.Fatal("readonly removed history", e)
		}
	}
	c.persistenceReadOnly = false
	if e := ClearWorkspaceHistoryContext(ctx); e != nil {
		t.Fatal(e)
	}
	for _, dir := range dirs {
		if _, e := os.Stat(dir); !os.IsNotExist(e) {
			t.Fatal("history retained", dir, e)
		}
	}
	if sql.CaptureLegacyHistoryQueue().DB != queue.DB {
		t.Fatal("clear replaced selected DB")
	}
	var count int
	if e := queue.DB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
	raw, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(raw, before) {
		t.Fatal("history clear changed document", e)
	}
	if e = ClearWorkspaceHistoryContext(ctx); e != nil {
		t.Fatal("repeat clear", e)
	}
}
