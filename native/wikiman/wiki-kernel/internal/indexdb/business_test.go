//go:build fts5

package indexdb

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOriginalReferenceAggregatesExplicitOwner(t *testing.T) {
	ctx := context.Background()
	a, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	da, _ := a.Database(Main)
	db, _ := b.Database(Main)
	for _, row := range [][3]string{{"same-id", "root", "ref1"}, {"same-id", "root", "ref1"}, {"other'quoted", "root", "ref2"}} {
		if _, err := da.Exec("INSERT INTO refs(def_block_id,def_block_root_id,block_id) VALUES(?,?,?)", row[0], row[1], row[2]); err != nil {
			t.Fatal(err)
		}
	}
	q := References{da}
	large := make([]string, 33000)
	for i := range large {
		large[i] = "same-id"
	}
	if got, err := q.DefinitionCounts(ctx, large); err != nil || got["same-id"] != 2 {
		t.Fatal("large/repeated IDs changed IN semantics", got, err)
	}
	counts, err := q.DefinitionCounts(ctx, []string{"same-id", "other'quoted"})
	if err != nil || counts["same-id"] != 2 || counts["other'quoted"] != 1 {
		t.Fatal(counts, err)
	}
	children, err := q.RootChildrenCounts(ctx, "root")
	if err != nil || len(children) != 2 {
		t.Fatal(children, err)
	}
	roots, err := q.RootCounts(ctx)
	if err != nil || roots["root"] != 2 {
		t.Fatal(roots, err)
	}
	duplicates, err := q.DuplicatedDefinitionRoots(ctx)
	if err != nil || len(duplicates) != 1 || duplicates[0] != "root" {
		t.Fatal(duplicates, err)
	}
	other, err := (References{db}).RootCounts(ctx)
	if err != nil || len(other) != 0 {
		t.Fatal(other, err)
	}
	if counts, err = q.DefinitionCounts(ctx, nil); err != nil || len(counts) != 0 {
		t.Fatal(counts, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = q.RootCounts(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = (References{}).RootCounts(ctx); !errors.Is(err, ErrNoDatabase) {
		t.Fatal(err)
	}
	da.Close()
	if _, err = q.RootCounts(ctx); err == nil {
		t.Fatal("closed connection accepted")
	}
}

func TestOriginalHistoryQueueBatchOwnershipRetryAndClose(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(History)
	var q HistoryQueue
	items := make([]HistoryEntry, 513)
	for i := range items {
		items[i] = HistoryEntry{ID: "same-id", Title: "original", Created: "20261021"}
	}
	if err = q.Index(items); err != nil {
		t.Fatal(err)
	}
	items[0].Title = "mutation"
	if err = q.Flush(ctx, nil, nil); !errors.Is(err, ErrNoDatabase) {
		t.Fatal(err)
	}
	var notifications []HistoryProgress
	if err = q.Flush(ctx, db, func(p HistoryProgress) { notifications = append(notifications, p) }); err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 2 || notifications[1].Current != 1 || notifications[1].Total != 1 {
		t.Fatal(notifications)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive WHERE title='original'").Scan(&count); err != nil || count != 513 {
		t.Fatal(count, err)
	}
	if err = q.DeleteBefore(20261022); err != nil {
		t.Fatal(err)
	}
	if err = q.Index([]HistoryEntry{{ID: "after-purge", Created: "20261023"}}); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = q.Flush(cancelled, db, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = q.Close(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
	if err = q.Index([]HistoryEntry{{ID: "late"}}); !errors.Is(err, ErrHistoryQueueClosed) {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err = q.Close(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
}

func TestOriginalHistoryQueueRollsBackWholeOperation(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(History)
	var q HistoryQueue
	items := make([]HistoryEntry, 513)
	for i := range items {
		items[i].ID = "same-id"
	}
	if err = q.Index(items); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	if err = q.Flush(cancelled, db, func(HistoryProgress) { cancel() }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if err = q.Flush(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&count); err != nil || count != 513 {
		t.Fatal(count, err)
	}
}

func TestOriginalHistoryQueueCloseDeadlineAndPanicRetry(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(History)
	var q HistoryQueue
	if err = q.Index([]HistoryEntry{{ID: "same-id"}}); err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() { done <- q.Flush(ctx, db, func(HistoryProgress) { close(entered); <-release }) }()
	<-entered
	deadline, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err = q.Close(deadline, db, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err = q.DeleteBefore(1); !errors.Is(err, ErrHistoryQueueClosed) {
		t.Fatal("close did not stop admission", err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = q.Close(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
	var p HistoryQueue
	if err = p.Index([]HistoryEntry{{ID: "panic-id"}}); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("observer panic swallowed")
			}
		}()
		_ = p.Flush(ctx, db, func(HistoryProgress) { panic("synthetic") })
	}()
	if err = p.Flush(ctx, db, nil); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive WHERE id='panic-id'").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
