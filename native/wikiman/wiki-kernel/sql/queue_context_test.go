//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import (
	"context"
	stdsql "database/sql"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"time"
	"wiki-kernel/internal/indexdb"
)

func mainQueueFixture(t *testing.T) *stdsql.DB {
	t.Helper()
	b, e := indexdb.OpenMemory(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	owner, _ := b.Database(indexdb.Main)
	oldDB, oldCase := db, caseSensitive
	dbQueueLock.Lock()
	oldQueue := operationQueue
	oldFailure, oldFailureDB := lastQueueFailure, lastQueueFailureDB
	lastQueueFailure = nil
	lastQueueFailureDB = nil
	operationQueue = nil
	dbQueueLock.Unlock()
	db, caseSensitive = owner, false
	t.Cleanup(func() {
		dbQueueLock.Lock()
		operationQueue = oldQueue
		lastQueueFailure, lastQueueFailureDB = oldFailure, oldFailureDB
		dbQueueLock.Unlock()
		db, caseSensitive = oldDB, oldCase
		b.Close()
		ClearCache()
	})
	e = indexdb.Transaction(context.Background(), owner, func(tx *stdsql.Tx) error {
		return indexdb.InsertBlocks(context.Background(), tx, []indexdb.BlockRecord{{ID: "a", RootID: "a", Type: "d", Content: "old"}, {ID: "b", RootID: "b", Type: "d", Content: "old"}, {ID: "c", RootID: "c", Type: "d", Content: "old"}}, false, nil)
	})
	if e != nil {
		t.Fatal(e)
	}
	return owner
}
func queuePending() int { dbQueueLock.Lock(); defer dbQueueLock.Unlock(); return len(operationQueue) }
func readContent(t *testing.T, d *stdsql.DB, id string) string {
	t.Helper()
	var v string
	if e := d.QueryRow("SELECT content FROM blocks WHERE id=?", id).Scan(&v); e != nil {
		t.Fatal(e)
	}
	return v
}
func TestMainQueueFailurePreservesTailAndCommittedPrefix(t *testing.T) {
	d := mainQueueFixture(t)
	if _, e := d.Exec("CREATE TABLE audit(id);CREATE TRIGGER audit_change AFTER UPDATE ON blocks BEGIN INSERT INTO audit VALUES(new.id); END;CREATE TRIGGER fail_update BEFORE UPDATE ON blocks WHEN new.content='bad' BEGIN SELECT RAISE(ABORT,'synthetic failure'); END"); e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct{ id, value string }{{"a", "good"}, {"b", "bad"}, {"c", "new"}} {
		UpdateBlockContentQueue(&Block{ID: v.id, Content: v.value})
	}
	if e := FlushQueueContext(context.Background()); e == nil {
		t.Fatal("failed transaction reported drained")
	}
	if e := WaitFlushTxContext(context.Background()); e == nil {
		t.Fatal("failed retained work caused false completion")
	}
	if queuePending() != 2 || readContent(t, d, "a") != "good" || readContent(t, d, "b") != "old" || readContent(t, d, "c") != "old" {
		t.Fatal("queue loss/rollback")
	}
	if _, e := d.Exec("DROP TRIGGER fail_update"); e != nil {
		t.Fatal(e)
	}
	if e := FlushQueueContext(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := WaitFlushTxContext(context.Background()); e != nil {
		t.Fatal("successful retry left failure latched", e)
	}
	var n int
	d.QueryRow("SELECT COUNT(*) FROM audit WHERE id='a'").Scan(&n)
	if n != 1 || queuePending() != 0 || readContent(t, d, "b") != "bad" || readContent(t, d, "c") != "new" {
		t.Fatal("committed prefix replayed or tail lost", n)
	}
}
func TestMainQueueCanceledWaitAndActiveTransactionActualJoin(t *testing.T) {
	d := mainQueueFixture(t)
	UpdateBlockContentQueue(&Block{ID: "a", Content: "accepted"})
	txLock <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	e := FlushQueueContext(ctx)
	cancel()
	<-txLock
	if !errors.Is(e, context.DeadlineExceeded) || queuePending() != 1 {
		t.Fatal(e, queuePending())
	}
	d.SetMaxOpenConns(2) // the in-memory bundle retains one keeper connection
	conn, e := d.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	active, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan error, 1)
	go func() { done <- FlushQueueContext(active) }()
	deadline := time.Now().Add(time.Second)
	for !flushingTx.Load() {
		if time.Now().After(deadline) {
			conn.Close()
			t.Fatal("flush not admitted")
		}
		time.Sleep(time.Millisecond)
	}
	stop()
	select {
	case e := <-done:
		conn.Close()
		t.Fatal("false completion of admitted DB work", e)
	default:
	}
	conn.Close()
	if e = <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if queuePending() != 0 || readContent(t, d, "a") != "accepted" {
		t.Fatal("committed work requeued")
	}
}
func TestMainQueueOwnerIdentityAndCoalescingDoNotCrossDatabases(t *testing.T) {
	a := mainQueueFixture(t)
	q := CaptureLegacyMainQueue()
	UpdateBlockContentQueue(&Block{ID: "a", Content: "owner-a"})
	bundle, e := indexdb.OpenMemory(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	b, _ := bundle.Database(indexdb.Main)
	db = b
	UpdateBlockContentQueue(&Block{ID: "a", Content: "owner-b"})
	if queuePending() != 2 {
		t.Fatal("new owner coalesced old queue")
	}
	if e = FlushQueueContext(context.Background()); !errors.Is(e, ErrMainQueueOwner) || queuePending() != 2 {
		t.Fatal(e)
	}
	db = a
	if e = q.Flush(context.Background()); !errors.Is(e, ErrMainQueueOwner) || queuePending() != 1 {
		t.Fatal("foreign tail not retained", e)
	}
	if readContent(t, a, "a") != "owner-a" {
		t.Fatal("wrong owner")
	}
	db = b
	if e = FlushQueueContext(context.Background()); e != nil || queuePending() != 0 {
		t.Fatal(e)
	}
}
func TestMainQueueBeginAndPanicFailuresRetainAcceptedOperation(t *testing.T) {
	d := mainQueueFixture(t)
	// The original index action would panic on a nil tree; convert to failure,
	// roll back and retain the entry rather than losing the whole batch.
	dbQueueLock.Lock()
	operationQueue = append(operationQueue, &dbQueueOperation{ownerDB: d, action: "index"})
	dbQueueLock.Unlock()
	if e := FlushQueueContext(context.Background()); e == nil || queuePending() != 1 {
		t.Fatal(e)
	}
	ClearQueue()
	UpdateBlockContentQueue(&Block{ID: "a", Content: "pending"})
	d.Close()
	if e := FlushQueueContext(context.Background()); e == nil || queuePending() != 1 {
		t.Fatal("begin failure discarded operation", e)
	}
}

func TestMainQueueCapturedUpsertAdmissionAndOriginalCoalescing(t *testing.T) {
	owner := mainQueueFixture(t)
	q := CaptureLegacyMainQueue()
	tree := &parse.Tree{ID: "root", Root: &ast.Node{ID: "root", Type: ast.NodeDocument}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.Upsert(ctx, tree); !errors.Is(err, context.Canceled) || queuePending() != 0 {
		t.Fatal("canceled admission", err)
	}
	foreign := LegacyMainQueue{}
	if err := foreign.Upsert(context.Background(), tree); !errors.Is(err, ErrMainQueueOwner) || queuePending() != 0 {
		t.Fatal("foreign admission", err)
	}
	if err := q.Upsert(context.Background(), tree); err != nil {
		t.Fatal(err)
	}
	replacement := &parse.Tree{ID: "root", Root: &ast.Node{ID: "root", Type: ast.NodeDocument}}
	if err := q.Upsert(context.Background(), replacement); err != nil {
		t.Fatal(err)
	}
	dbQueueLock.Lock()
	if len(operationQueue) != 1 || operationQueue[0].ownerDB != owner || operationQueue[0].upsertTree != replacement {
		t.Error("original owner-local coalescing changed")
	}
	operationQueue = nil
	dbQueueLock.Unlock()
}

func TestMainQueueCapturedIndexAdmissionAndOriginalCoalescing(t *testing.T) {
	owner := mainQueueFixture(t)
	q := CaptureLegacyMainQueue()
	tree := &parse.Tree{ID: "root", Root: &ast.Node{ID: "root", Type: ast.NodeDocument}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.Index(ctx, tree); !errors.Is(err, context.Canceled) || queuePending() != 0 {
		t.Fatal(err)
	}
	if err := (LegacyMainQueue{}).Index(context.Background(), tree); !errors.Is(err, ErrMainQueueOwner) || queuePending() != 0 {
		t.Fatal(err)
	}
	if err := q.Index(context.Background(), nil); !errors.Is(err, ErrMainQueueOperation) {
		t.Fatal(err)
	}
	if err := q.Index(context.Background(), tree); err != nil {
		t.Fatal(err)
	}
	replacement := &parse.Tree{ID: "root", Root: &ast.Node{ID: "root", Type: ast.NodeDocument}}
	if err := q.Index(context.Background(), replacement); err != nil {
		t.Fatal(err)
	}
	dbQueueLock.Lock()
	if len(operationQueue) != 1 || operationQueue[0].ownerDB != owner || operationQueue[0].indexTree != replacement || operationQueue[0].action != "index" {
		t.Error("captured index coalescing changed")
	}
	operationQueue = nil
	dbQueueLock.Unlock()
}
