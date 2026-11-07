//go:build fts5

package workspace

import (
	"context"
	"errors"
	"testing"
	"time"
	"wiki-kernel/internal/indexdb"
)

func TestScopedMainMutationBackgroundAndOriginalStateCallbacks(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
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
	for _, l := range []*Lease{a, b} {
		if err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
			db, _ := bundle.Database(indexdb.Main)
			for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
				if _, err := db.Exec("INSERT INTO " + table + "(id,root_id,box,path,content) VALUES('same-id','same-root','box','/a.docj','original')"); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan IndexEvent, 16)
	sub, err := a.SubscribeIndexEvents(ctx, func(e IndexEvent) {
		data, err := a.Configuration(ctx)
		if err != nil {
			t.Error(err)
		}
		expected := 0
		if e.Kind == "main-index-changed" {
			expected = 1
		}
		if data.DataIndexState != expected {
			t.Errorf("state %d want %d on %s", data.DataIndexState, expected, e.Kind)
		}
		events <- e
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close(ctx)
	if err = a.QueueNativeMainMutation(ctx, indexdb.MainMutation{Action: "delete_id", ID: "same-root"}); err != nil {
		t.Fatal(err)
	}
	if e := <-events; e.Kind != "main-index-changed" {
		t.Fatal(e)
	}
	task, err := a.StartNativeMainFlush(ctx, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-events:
		if e.Kind != "main-index-flushed" || e.Action != "delete_id" || e.PageID != pageA || e.Generation != a.workspace.generation {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("no real background mutation")
	}
	task.Cancel()
	if err = task.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for l, expected := range map[*Lease]int{a: 0, b: 1} {
		if err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
			db, _ := bundle.Database(indexdb.Main)
			var count int
			err := db.QueryRow("SELECT COUNT(*) FROM blocks").Scan(&count)
			if count != expected {
				t.Errorf("wrong owner %d want %d", count, expected)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := b.Configuration(ctx)
	if err != nil || data.DataIndexState != 0 {
		t.Fatal("B state changed", data, err)
	}
	if digest(t, root) != before {
		t.Fatal("derived mutations changed source")
	}
}
