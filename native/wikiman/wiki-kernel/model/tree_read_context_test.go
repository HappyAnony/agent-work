//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

func TestTreeReadActualReindexReadOnlyAndSourceUnchanged(t *testing.T) {
	c, r, def, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	ctx := context.Background()
	const root = "20261028140003-refdoc2"
	info, e := GetBlockInfoContext(ctx, "20261028140004-refpar1")
	if e != nil || info["rootID"] != root || info["rootTitle"] != "reference fixture" || info["rootChildID"] != root {
		t.Fatal(info, e)
	}
	if _, e = r.registry.Remove(ctx, indexdb.BlockTreeFilter{Kind: "root", Value: root}); e != nil {
		t.Fatal(e)
	}
	if _, e = r.main.DB.Exec("DELETE FROM blocks WHERE root_id=?", root); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadTreeByBlockIDContext(ctx, root); !errors.Is(e, ErrTreeNotFound) {
		t.Fatal(e)
	}
	tree, e := LoadTreeByBlockIDWithReindexContext(ctx, root)
	if e != nil || tree == nil || tree.ID != root {
		t.Fatal(tree, e)
	}
	rows, e := r.main.Blocks(ctx, []string{root})
	if e != nil || len(rows) != 1 || rows[0] == nil {
		t.Fatal("derived queue not drained", rows, e)
	}
	if tree, e = LoadTreeByBlockIDContext(ctx, def); e != nil || tree == nil {
		t.Fatal(tree, e)
	}
	raw, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(before, raw) {
		t.Fatal("reindex changed source", e)
	}
	if e = os.WriteFile(file, []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	if tree, e = LoadTreeByBlockIDContext(ctx, root); e == nil || tree != nil {
		t.Fatal("source failure succeeded", tree, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if tree, e = LoadTreeByBlockIDWithReindexContext(canceled, root); !errors.Is(e, context.Canceled) || tree != nil {
		t.Fatal(tree, e)
	}
}
