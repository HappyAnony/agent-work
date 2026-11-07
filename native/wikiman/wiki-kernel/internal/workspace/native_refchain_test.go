//go:build fts5

package workspace

import (
	"context"
	"database/sql"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestSourceCacheRefAndUpperQueryUseOwnerAndCommitInvalidation(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	a, e := m.Acquire(ctx, storeID, pageA)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Release()
	b, e := m.Acquire(ctx, storeID, pageB)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Release()
	const ref = "20261019090004-ref0001"
	if e = a.CacheNativeRef(ctx, boxID, docID, ref, 0); e != nil {
		t.Fatal(e)
	}
	rows, e := a.NativeRefsCacheByDefinition(ctx, blockID)
	if e != nil || len(rows) != 1 || rows[0].DefBlockParentID != docID || rows[0].BlockID != ref {
		t.Fatal(rows, e)
	}
	rows[0].Content = "outside"
	rows, e = b.NativeRefsCacheByDefinition(ctx, blockID)
	if e != nil || len(rows) != 0 {
		t.Fatal("A cache leaked to B", rows, e)
	}
	if e = a.QueueNativeReferences(ctx, boxID, docID); e != nil {
		t.Fatal(e)
	}
	if e = a.FlushNativeMainQueue(ctx); e != nil {
		t.Fatal(e)
	}
	rows, e = a.NativeRefsCacheByDefinition(ctx, blockID)
	if e != nil || len(rows) != 1 || rows[0].Content != "fixture paragraph" {
		t.Fatal(rows, e)
	}
	if e = a.WithNativeReferences(ctx, func(q indexdb.References) error {
		r, e := q.ByPair(ctx, blockID, ref)
		if e != nil || len(r) != 1 {
			t.Fatal(r, e)
		}
		ids, e := q.ChildDefinitionIDs(ctx, docID)
		if e != nil || len(ids) != 1 {
			t.Fatal(ids, e)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if e = a.QueueNativeMainMutation(ctx, indexdb.MainMutation{Action: "delete_refs", Box: boxID, Path: "/" + docID + ".docj"}); e != nil {
		t.Fatal(e)
	}
	if e = a.FlushNativeMainQueue(ctx); e != nil {
		t.Fatal(e)
	}
	rows, e = a.NativeRefsCacheByDefinition(ctx, blockID)
	if e != nil || len(rows) != 0 {
		t.Fatal("committed deletion retained ref cache", rows, e)
	}
	if digest(t, root) != before {
		t.Fatal("queries/cache changed source")
	}
	if e = a.WithNativeTransaction(ctx, indexdb.Main, func(tx *sql.Tx) error {
		return indexdb.InsertBlockReferences(ctx, tx, []indexdb.RefRecord{{ID: "1", DefBlockID: blockID, BlockID: "one"}, {ID: "2", DefBlockID: blockID, BlockID: "two"}})
	}); e != nil {
		t.Fatal(e)
	}
	if e = a.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { a.workspace.resources.nativeRefs.Limit = 1; return nil }); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		rows, e = a.NativeRefsCacheByDefinition(ctx, blockID)
		if e != nil || len(rows) != 2 {
			t.Fatal("cache budget truncated authoritative query", rows, e)
		}
	}
}
