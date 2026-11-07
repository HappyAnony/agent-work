//go:build fts5

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func TestReferenceSearchActualRecentKeywordIgnoreAndSQLFailure(t *testing.T) {
	c, r, id, _, _ := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	ctx := context.Background()
	using := "20261028140004-refpar1"
	root := "20261028140003-refdoc2"
	for _, query := range []string{"", "definition", "block " + id} {
		blocks, newDoc, e := SearchRefBlockContext(ctx, using, root, query, 12, false, false)
		if e != nil || len(blocks) != 1 || blocks[0].ID != id || blocks[0].RefText != "definition" || blocks[0].RefCount != 1 {
			t.Fatal(query, blocks, newDoc, e)
		}
	}
	blocks, _, e := SearchRefBlockContext(ctx, id, "20261028140001-refdoc1", "definition", 12, false, false)
	if e != nil {
		t.Fatal(e)
	}
	for _, b := range blocks {
		if b.ID == id || b.ID == "20261028140001-refdoc1" {
			t.Fatal("self not filtered", b)
		}
	}
	dir := filepath.Join(util.DataDir, ".siyuan")
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(dir, "refsearchignore"), []byte("b.id != '"+id+"'"), 0600); e != nil {
		t.Fatal(e)
	}
	blocks, _, e = SearchRefBlockContext(ctx, using, root, "", 12, false, false)
	if e != nil || len(blocks) != 0 {
		t.Fatal(blocks, e)
	}
	if _, e = r.main.DB.Exec("DROP TABLE blocks_fts_case_insensitive"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = SearchRefBlockContext(ctx, using, root, "definition", 12, false, false); e == nil {
		t.Fatal("SQL error swallowed")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, e = SearchRefBlockContext(canceled, using, root, "", 12, false, false); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
