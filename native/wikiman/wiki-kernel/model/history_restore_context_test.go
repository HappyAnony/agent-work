//go:build fts5

package model

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func TestHistoryRestoreAssetActualSourceBoundaryAndReadOnly(t *testing.T) {
	c, _, _, _, _ := referenceModelFixture(t)
	ctx := context.Background()
	name := filepath.Join(util.HistoryDir, "stamp", "assets", "file.txt")
	if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(name, []byte("restored asset"), 0600); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if e := RollbackAssetsHistoryContext(ctx, name); e == nil {
		t.Fatal("readonly restored")
	}
	c.persistenceReadOnly = false
	if e := RollbackAssetsHistoryContext(ctx, name); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(util.DataDir, "assets", "file.txt")
	raw, e := os.ReadFile(target)
	if e != nil || string(raw) != "restored asset" {
		t.Fatal(string(raw), e)
	}
	if c.syncRuntime.plan.IsZero() {
		t.Fatal("sync not scheduled")
	}
	if e = RollbackAssetsHistoryContext(ctx, filepath.Join(t.TempDir(), "outside.txt")); e == nil {
		t.Fatal("outside owner history accepted")
	}
	if e = os.WriteFile(name, []byte("replacement asset"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = RollbackAssetsHistoryContext(ctx, name); e != nil {
		t.Fatal(e)
	}
	raw, e = os.ReadFile(target)
	if e != nil || string(raw) != "replacement asset" {
		t.Fatal(string(raw), e)
	}
}

func TestHistoryRestoreNotebookActualCopyAndJoinedFullIndex(t *testing.T) {
	c, _, _, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.Editor.VirtualBlockRef = false
	ctx := context.Background()
	box := filepath.Base(filepath.Dir(file))
	folder := filepath.Join(util.HistoryDir, "2026-11-04-120000-delete", box)
	if e := os.MkdirAll(folder, 0700); e != nil {
		t.Fatal(e)
	}
	restored := []byte(strings.ReplaceAll(string(before), "definition", "restored definition"))
	if e := os.WriteFile(filepath.Join(folder, filepath.Base(file)), restored, 0600); e != nil {
		t.Fatal(e)
	}
	if e := RollbackNotebookHistoryContext(ctx, folder); e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(file)
	if e != nil || bytes.Equal(after, before) || !bytes.Contains(after, []byte("restored definition")) {
		t.Fatal(string(after), e)
	}
	db := sql.CaptureLegacyRepairQueries().DB
	var content string
	if e = db.QueryRow("SELECT content FROM blocks WHERE id=?", "20261028140004-refpar1").Scan(&content); e != nil || !strings.Contains(content, "restored definition") {
		t.Fatal("restore returned before index", content, e)
	}
}
