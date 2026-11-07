//go:build fts5

package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/util"
)

func TestNotebookHistoryActualReadOnlyAndCorruption(t *testing.T) {
	c, _, _, _, _ := referenceModelFixture(t)
	c.persistenceReadOnly = true
	name := filepath.Join(util.HistoryDir, "2026-11-04-120000-delete", "20261028140000-refbox1", ".siyuan", "conf.json")
	if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(name, []byte(`{"name":"Deleted notebook"}`), 0600); e != nil {
		t.Fatal(e)
	}
	out, e := GetNotebookHistoryContext(context.Background())
	if e != nil || len(out) != 1 || out[0].Items[0].Title != "Deleted notebook" || out[0].Items[0].Path != filepath.Dir(filepath.Dir(name)) {
		t.Fatal(out, e)
	}
	if e = os.WriteFile(name, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if out, e = GetNotebookHistoryContext(context.Background()); e == nil || out != nil {
		t.Fatal("corruption returned success", out, e)
	}
}
