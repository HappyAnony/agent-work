//go:build fts5

package model

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/util"
)

func TestNotebookRemoveActualBackupAssetsAndJoinedIndex(t *testing.T) {
	c, r, id, file, before := notebookLifecycleFixture(t)
	ctx := context.Background()
	asset := filepath.Join(util.DataDir, id, "assets/kept.txt")
	if e := os.MkdirAll(filepath.Dir(asset), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(asset, []byte("kept bytes"), 0600); e != nil {
		t.Fatal(e)
	}
	info, e := NotebookInfoContext(ctx, id)
	if e != nil || info.DocCount != 2 || info.Size == 0 {
		t.Fatal(info, e)
	}
	c.persistenceReadOnly = true
	if e = RemoveBoxContext(ctx, id); e == nil {
		t.Fatal("readonly removed notebook")
	}
	c.persistenceReadOnly = false
	if e = RemoveBoxContext(ctx, id); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(util.DataDir, id)); !os.IsNotExist(e) {
		t.Fatal("notebook not removed", e)
	}
	promoted, e := os.ReadFile(filepath.Join(util.DataDir, "assets/kept.txt"))
	if e != nil || string(promoted) != "kept bytes" {
		t.Fatal("notebook assets lost", e)
	}
	matches, e := filepath.Glob(filepath.Join(util.HistoryDir, "*-delete", id, filepath.Base(file)))
	if e != nil || len(matches) != 1 {
		t.Fatal(matches, e)
	}
	backup, e := os.ReadFile(matches[0])
	if e != nil || !bytes.Equal(backup, before) {
		t.Fatal("backup lost original", e)
	}
	var count int
	if e = r.main.DB.QueryRow("SELECT count(*) FROM blocks WHERE box=?", id).Scan(&count); e != nil || count != 0 {
		t.Fatal("notebook indexes not drained", count, e)
	}
	if e = RemoveBoxContext(ctx, id); e != nil {
		t.Fatal("missing removal not idempotent", e)
	}
}
