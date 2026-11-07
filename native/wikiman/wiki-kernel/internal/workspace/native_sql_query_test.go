//go:build fts5

package workspace

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestNativeSQLQueryFixedIndexesAndNoWriteCapability(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, lifecycleOptions(root))
	ctx := context.Background()
	for _, page := range []string{pageA, pageB} {
		l, e := m.Acquire(ctx, storeID, page)
		if e != nil {
			t.Fatal(e)
		}
		if e = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
			db, e := b.Database(indexdb.Main)
			if e != nil {
				return e
			}
			_, e = db.Exec("CREATE TABLE query_owner (value TEXT); INSERT INTO query_owner VALUES (?)", page)
			return e
		}); e != nil {
			t.Fatal(e)
		}
		out, e := l.NativeSQLQuery(ctx, "SELECT value FROM query_owner")
		if e != nil || len(out) != 1 || out[0]["value"] != page {
			t.Fatal(out, e)
		}
		if _, e = l.NativeSQLQuery(ctx, "DELETE FROM query_owner"); e == nil {
			t.Fatal("native query acquired writer")
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if out, e = l.NativeSQLQuery(cancelled, "SELECT value FROM query_owner"); !errors.Is(e, context.Canceled) || out != nil {
			t.Fatal(out, e)
		}
		l.Release()
	}
	if digest(t, root) != before {
		t.Fatal("SQL reader changed source")
	}
}
