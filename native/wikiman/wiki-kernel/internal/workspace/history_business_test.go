//go:build fts5

package workspace

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalHistoryBackgroundAndReferencesUseCreationLease(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	ctx := context.Background()
	m := open(t, root, Options{})
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	b, err := m.Acquire(ctx, storeID, pageB)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	for lease, title := range map[*Lease]string{a: "A", b: "B"} {
		if err = lease.QueueNativeHistories(ctx, []indexdb.HistoryEntry{{ID: "same-id", Title: title, Created: "20261021"}}); err != nil {
			t.Fatal(err)
		}
		if err = lease.WithNativeTransaction(ctx, indexdb.Main, func(tx *sql.Tx) error {
			_, err := tx.Exec("INSERT INTO refs(def_block_id,def_block_root_id,block_id) VALUES('same-id',?,'ref')", title)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	task, err := a.StartNativeHistoryFlush(ctx, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		count := 0
		err = a.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
			db, _ := bundle.Database(indexdb.History)
			return db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive WHERE title='A'").Scan(&count)
		})
		if err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual background flush did not run")
		}
		time.Sleep(time.Millisecond)
	}
	if err = b.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, _ := bundle.Database(indexdb.History)
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("A worker flushed B queue")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = b.FlushNativeHistories(ctx); err != nil {
		t.Fatal(err)
	}
	for lease, title := range map[*Lease]string{a: "A", b: "B"} {
		if err = lease.WithNativeReferences(ctx, func(q indexdb.References) error {
			roots, err := q.RootCounts(ctx)
			if roots[title] != 1 || len(roots) != 1 {
				t.Fatal(roots)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	task.Cancel()
	if err = task.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if digest(t, root) != before {
		t.Fatal("derived history flush changed authoritative source")
	}
}
