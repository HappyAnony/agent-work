//go:build fts5

package workspace

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestNativeIndexVacuumFixedOwnerAndSourceUnchanged(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, lifecycleOptions(root))
	ctx := context.Background()
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
	for _, l := range []*Lease{a, b} {
		if e = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
			db, e := bundle.Database(indexdb.Main)
			if e != nil {
				return e
			}
			_, e = db.Exec("CREATE TABLE vacuum_fixture (data BLOB)")
			if e != nil {
				return e
			}
			for i := 0; i < 16; i++ {
				if _, e = db.Exec("INSERT INTO vacuum_fixture VALUES (zeroblob(16384))"); e != nil {
					return e
				}
			}
			_, e = db.Exec("DELETE FROM vacuum_fixture")
			return e
		}); e != nil {
			t.Fatal(e)
		}
	}
	sizes, e := a.VacuumNativeIndexes(ctx)
	if e != nil || len(sizes) != 3 {
		t.Fatal(sizes, e)
	}
	for i, l := range []*Lease{a, b} {
		if e = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
			db, e := bundle.Database(indexdb.Main)
			if e != nil {
				return e
			}
			var free int
			if e = db.QueryRow("PRAGMA freelist_count").Scan(&free); e != nil {
				return e
			}
			if i == 0 && free != 0 || i == 1 && free == 0 {
				t.Fatal("vacuum used another owner", i, free)
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if out, e := a.VacuumNativeIndexes(canceled); !errors.Is(e, context.Canceled) || out != nil {
		t.Fatal(out, e)
	}
	if digest(t, root) != before {
		t.Fatal("derived maintenance touched source")
	}
}
