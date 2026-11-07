//go:build fts5

package sql

import (
	"context"
	"database/sql"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalReferenceFacadesUseSharedAlgorithmsAndCopyCache(t *testing.T) {
	ctx := context.Background()
	bundle, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	original := db
	db, _ = bundle.Database(indexdb.Main)
	defer func() { db = original }()
	e = indexdb.Transaction(ctx, db, func(tx *sql.Tx) error {
		if e := indexdb.InsertBlocks(ctx, tx, []indexdb.BlockRecord{{ID: "root", Type: "d", Content: "doc title"}, {ID: "def", ParentID: "root", Type: "p", Content: "text"}, {ID: "ref", RootID: "root", Type: "p"}}, false, nil); e != nil {
			return e
		}
		return indexdb.InsertBlockReferences(ctx, tx, []indexdb.RefRecord{{ID: "ref-row", DefBlockID: "def", DefBlockRootID: "root", BlockID: "ref", RootID: "root", Content: "anchor"}})
	})
	if e != nil {
		t.Fatal(e)
	}
	if r := QueryRefsByDefID("root", true); len(r) != 1 || r[0].Content != "anchor" {
		t.Fatal(r)
	}
	if r := QueryRefsByDefIDRefID("def", "ref"); len(r) != 1 {
		t.Fatal(r)
	}
	if r := QueryDefRootBlocksByRefRootID("root"); len(r) != 1 || r[0].Content != "doc title" {
		t.Fatal(r)
	}
	if r := QueryRefRootBlocksByDefRootIDs([]string{"root"}); len(r["root"]) != 1 {
		t.Fatal(r)
	}
	if r := QueryChildDefIDsByRootDefID("root"); len(r) != 1 {
		t.Fatal(r)
	}
	if r := QueryRefIDsByDefID("root", true); len(r) != 1 {
		t.Fatal(r)
	}
	if r := QueryRefsRecent(false, "('p')", nil); len(r) != 1 {
		t.Fatal(r)
	}
	if r := QueryBlockDefIDsByRefText("anchor", nil); len(r) != 1 || r[0] != "def" {
		t.Fatal(r)
	}
	if r := DefRefs("", -1); len(r) != 1 {
		t.Fatal(r)
	}
	if text := GetRefText("def"); text != "text" {
		t.Fatal(text)
	}
	if e = OpenLegacyCaches(); e != nil {
		t.Fatal(e)
	}
	defer CloseLegacyCaches()
	clearAllCaches()
	r := GetRefsCacheByDefID("def")
	if len(r) != 1 {
		t.Fatal(r)
	}
	r[0].Content = "outside"
	if GetRefsCacheByDefID("def")[0].Content != "anchor" {
		t.Fatal("cache alias")
	}
}
