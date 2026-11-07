//go:build fts5

package model

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestAssetCleanActualHistoryAndReadonly(t *testing.T) {
	c, _, _, _, _ := referenceModelFixture(t)
	c.Export = conf.NewExport()
	ctx := context.Background()
	file := filepath.Join(util.DataDir, "assets/unused.txt")
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(file, []byte("original asset"), 0600); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	if _, e := RemoveUnusedAssetContext(ctx, "assets/unused.txt"); e == nil {
		t.Fatal("readonly source removed")
	}
	c.persistenceReadOnly = false
	got, e := RemoveUnusedAssetContext(ctx, "assets/unused.txt")
	if e != nil || got != file {
		t.Fatal(got, e)
	}
	if _, e = os.Stat(file); !os.IsNotExist(e) {
		t.Fatal("source not removed", e)
	}
	matches, e := filepath.Glob(filepath.Join(util.HistoryDir, "*-clean/assets/unused.txt"))
	if e != nil || len(matches) != 1 {
		t.Fatal(matches, e)
	}
	raw, e := os.ReadFile(matches[0])
	if e != nil || string(raw) != "original asset" {
		t.Fatal(string(raw), e)
	}
	got, e = RemoveUnusedAssetContext(ctx, "assets/unused.txt")
	if e != nil || got != file {
		t.Fatal("missing idempotence", got, e)
	}
	if _, e = RemoveUnusedAssetContext(ctx, "../outside.txt"); e == nil {
		t.Fatal("escaped clean")
	}
}
