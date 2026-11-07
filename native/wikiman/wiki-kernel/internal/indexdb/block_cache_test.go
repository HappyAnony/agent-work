//go:build fts5

package indexdb

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"wiki-kernel/search"
)

func TestCachedBlocksActualSQLIsolationAndLifecycle(t *testing.T) {
	ctx := context.Background()
	a, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	da, _ := a.Database(Main)
	db, _ := b.Database(Main)
	seedMainMutation(t, da)
	seedMainMutation(t, db)
	if _, e = db.Exec("UPDATE blocks SET content='other owner'"); e != nil {
		t.Fatal(e)
	}
	ca, cb := &BlockCache{}, &BlockCache{}
	qa, qb := CachedBlocks{da, ca}, CachedBlocks{db, cb}
	rows, e := qa.GetMany(ctx, []string{"missing", "root'quoted", "root'quoted"})
	if e != nil {
		t.Fatal(e)
	}
	if rows[0] != nil || rows[1].Content != "before" || rows[1] == rows[2] {
		t.Fatal(rows)
	}
	rows[1].Content = "borrowed"
	row, ok, e := qa.Get(ctx, "root'quoted")
	if e != nil || !ok || row.Content != "before" {
		t.Fatal(row, e)
	}
	row, ok, e = qb.Get(ctx, "root'quoted")
	if e != nil || !ok || row.Content != "other owner" {
		t.Fatal(row, e)
	}
	if _, e = da.Exec("UPDATE blocks SET content='changed'"); e != nil {
		t.Fatal(e)
	}
	ca.Delete("root'quoted")
	row, _, e = qa.Get(ctx, "root'quoted")
	if e != nil || row.Content != "changed" {
		t.Fatal(row, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, e = qa.Get(canceled, "root'quoted"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	ca.Close()
	if _, _, e = qa.Get(ctx, "root'quoted"); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	if _, _, e = qb.Get(ctx, "root'quoted"); e != nil {
		t.Fatal("closing A affected B", e)
	}
}

func TestCachedBlocksBatchFailuresAndValueCache(t *testing.T) {
	ctx := context.Background()
	b, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	db, _ := b.Database(Main)
	seedMainMutation(t, db)
	ids := []string{"root'quoted"}
	for i := 0; i < 1100; i++ {
		ids = append(ids, fmt.Sprint(i))
	}
	c := &BlockCache{Limit: 2}
	q := CachedBlocks{db, c}
	rows, e := q.GetMany(ctx, ids)
	if e != nil || len(rows) != len(ids) || rows[0] == nil || rows[1100] != nil {
		t.Fatal(e)
	}
	c.Clear()
	if _, e = db.Exec("DROP TABLE blocks"); e != nil {
		t.Fatal(e)
	}
	if _, e = q.GetMany(ctx, ids); e == nil {
		t.Fatal("SQL error hidden")
	}
	if _, ok := c.Get(ids[0]); ok {
		t.Fatal("failed query populated cache")
	}
	c.Put(BlockRecord{ID: "a", Content: search.SearchMarkLeft + "clean" + search.SearchMarkRight})
	row, _ := c.Get("a")
	if row.Content != "clean" {
		t.Fatal(row)
	}
	c.Put(BlockRecord{ID: "b"})
	c.Put(BlockRecord{ID: "c"})
	if _, ok := c.Get("a"); ok {
		t.Fatal("capacity ignored")
	}
	c.Close()
	c.Put(BlockRecord{ID: "late"})
	if _, ok := c.Get("late"); ok {
		t.Fatal("closed cache reopened")
	}
}
