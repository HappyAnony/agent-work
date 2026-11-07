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

func TestScopedIndexBusinessEventsAndActualBackgroundWork(t *testing.T) {
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
	defer a.Release()
	defer b.Release()
	events := make(chan IndexEvent, 8)
	sub, err := a.SubscribeIndexEvents(ctx, func(event IndexEvent) {
		// Event delivery is outside the DB gate: a callback can read its owner.
		if _, err := a.Configuration(ctx); err != nil {
			t.Error(err)
		}
		events <- event
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close(ctx)
	otherEvents := 0
	other, err := b.SubscribeIndexEvents(ctx, func(IndexEvent) { otherEvents++ })
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close(ctx)
	for lease, name := range map[*Lease]string{a: "A", b: "B"} {
		if err = lease.QueueNativeAssetContents(ctx, []indexdb.AssetContentEntry{{ID: "same-id", Name: name, Path: "assets/a.pdf"}}); err != nil {
			t.Fatal(err)
		}
	}
	task, err := a.StartNativeAssetContentFlush(ctx, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.StoreID != storeID || event.PageID != pageA || event.Generation != a.workspace.generation || event.Kind != "asset-index-progress" {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("no real index event")
	}
	task.Cancel()
	if err = task.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if otherEvents != 0 {
		t.Fatal("event routed to another page")
	}
	if err = b.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, _ := bundle.Database(indexdb.Assets)
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM asset_contents_fts_case_insensitive").Scan(&count)
		if count != 0 {
			t.Error("A task flushed B")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err = a.QueueNativeHistories(ctx, []indexdb.HistoryEntry{{ID: "same-id"}}); err != nil {
		t.Fatal(err)
	}
	if err = a.FlushNativeHistories(ctx); err != nil {
		t.Fatal(err)
	}
	if event := <-events; event.Kind != "history-index-progress" || event.PageID != pageA {
		t.Fatal(event)
	}
	if err = a.QueueNativeBlockContent(ctx, "same-id", "new text"); err != nil {
		t.Fatal(err)
	}
	if event := <-events; event.Kind != "main-index-changed" {
		t.Fatal(event)
	}
	if err = a.FlushNativeBlockContents(ctx); err != nil {
		t.Fatal(err)
	}
	if event := <-events; event.Kind != "main-index-flushed" {
		t.Fatal(event)
	}
	if digest(t, root) != before {
		t.Fatal("derived index mutated source")
	}
}

func TestIndexCallbackPinsContextUntilActualReturn(t *testing.T) {
	ctx := context.Background()
	m := open(t, fixture(t), Options{})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.QueueNativeAssetContents(ctx, []indexdb.AssetContentEntry{{ID: "same-id", Name: "A"}}); err != nil {
		t.Fatal(err)
	}
	var db *sql.DB
	if err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		var err error
		db, err = bundle.Database(indexdb.Assets)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	sub, err := l.SubscribeIndexEvents(ctx, func(IndexEvent) { close(entered); <-release })
	if err != nil {
		t.Fatal(err)
	}
	task, err := l.StartNativeAssetContentFlush(ctx, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("callback did not start")
	}
	l.Release()
	deadline, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err = m.Drain(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		t.Fatal("DB closed while real callback was live", err)
	}
	close(release)
	if err = task.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = m.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if err = sub.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.Ping(); err == nil {
		t.Fatal("DB survived successful drain")
	}
}

func TestIndexSubscriptionContextCancellationStopsDelivery(t *testing.T) {
	m := open(t, fixture(t), Options{})
	l, err := m.Acquire(context.Background(), storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	sub, err := l.SubscribeIndexEvents(ctx, func(IndexEvent) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err = sub.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = l.workspace.resources.indexEvents.Publish(l.indexEvent("main-index-changed", "fixture", 0, 0)); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("cancelled request subscription received a later event", calls)
	}
}
