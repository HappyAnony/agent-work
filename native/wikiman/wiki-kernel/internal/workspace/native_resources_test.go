//go:build fts5

package workspace

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"wiki-kernel/internal/ialcache"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalResourcesBoundToWorkspaceLease(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Acquire(ctx, storeID, pageB)
	if err != nil {
		t.Fatal(err)
	}
	var first, other *indexdb.Bundle
	var db *sql.DB
	if err = a.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		first = bundle
		var err error
		db, err = bundle.Database(indexdb.Main)
		if err != nil {
			return err
		}
		_, err = db.Exec(`INSERT INTO blocks(id,content) VALUES('same-id','page A')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = b.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		other = bundle
		db, err := bundle.Database(indexdb.Main)
		if err != nil {
			return err
		}
		var count int
		if err = db.QueryRow(`SELECT count(*) FROM blocks WHERE id='same-id'`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("cross-page index")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if first == other {
		t.Fatal("shared bundle")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
				if bundle != first {
					t.Error("duplicate initialization")
				}
				return nil
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var cache *ialcache.Cache
	if err = a.WithNativeIAL(ctx, func(c *ialcache.Cache) error {
		cache = c
		c.PutBlock("same-id", map[string]string{"name": "A"})
		c.Wait()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = b.WithNativeIAL(ctx, func(c *ialcache.Cache) error {
		if c == cache || c.GetBlock("same-id") != nil {
			t.Fatal("cross-page cache")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if digest(t, root) != before {
		t.Fatal("resource init wrote authoritative source")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = m.Drain(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		t.Fatal("active lease's DB closed", err)
	}
	a.Release()
	b.Release()
	if err = m.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if db.Ping() == nil {
		t.Fatal("DB survived context close")
	}
	if cache.GetBlock("same-id") != nil {
		t.Fatal("cache survived context close")
	}
	if err = a.WithNativeIndexes(ctx, func(*indexdb.Bundle) error { return nil }); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}
