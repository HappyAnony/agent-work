package sql

import (
	"context"
	stdsql "database/sql"
	"errors"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
)

func admissionQueuePending() int {
	dbQueueLock.Lock()
	defer dbQueueLock.Unlock()
	return len(operationQueue)
}

func TestMainQueueCapturedBatchRemoveAdmissionAndIsolation(t *testing.T) {
	owner := &stdsql.DB{}
	oldDB := db
	dbQueueLock.Lock()
	oldQueue := operationQueue
	operationQueue = nil
	dbQueueLock.Unlock()
	db = owner
	t.Cleanup(func() {
		dbQueueLock.Lock()
		operationQueue = oldQueue
		dbQueueLock.Unlock()
		db = oldDB
	})

	q := CaptureLegacyMainQueue()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.BatchRemove(ctx, []string{"a"}); !errors.Is(err, context.Canceled) || admissionQueuePending() != 0 {
		t.Fatal("canceled admission", err)
	}
	if err := (LegacyMainQueue{}).BatchRemove(context.Background(), []string{"a"}); !errors.Is(err, ErrMainQueueOwner) || admissionQueuePending() != 0 {
		t.Fatal("foreign admission", err)
	}
	if err := q.BatchRemove(context.Background(), nil); err != nil || admissionQueuePending() != 0 {
		t.Fatal("empty admission", err)
	}
	ids := []string{"a", "b"}
	if err := q.BatchRemove(context.Background(), ids); err != nil {
		t.Fatal(err)
	}
	ids[0] = "changed"
	dbQueueLock.Lock()
	if len(operationQueue) != 1 || operationQueue[0].ownerDB != owner || operationQueue[0].action != "delete_ids" || operationQueue[0].removeTreeIDs[0] != "a" {
		t.Error("captured batch-remove admission changed")
	}
	operationQueue = nil
	dbQueueLock.Unlock()
}

func TestMainQueueCapturedNotebookAdmissionsAndReplacementIsolation(t *testing.T) {
	owner := &stdsql.DB{}
	oldDB := db
	dbQueueLock.Lock()
	oldQueue := operationQueue
	operationQueue = nil
	dbQueueLock.Unlock()
	db = owner
	t.Cleanup(func() {
		dbQueueLock.Lock()
		operationQueue = oldQueue
		dbQueueLock.Unlock()
		db = oldDB
	})

	q := CaptureLegacyMainQueue()
	tree := &parse.Tree{ID: "root", Root: &ast.Node{ID: "root", Type: ast.NodeDocument}}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.UpdateRefs(canceled, tree); !errors.Is(err, context.Canceled) || admissionQueuePending() != 0 {
		t.Fatal("canceled refs admission", err)
	}
	if err := q.UpdateRefs(context.Background(), nil); !errors.Is(err, ErrMainQueueOperation) || admissionQueuePending() != 0 {
		t.Fatal("invalid refs admission", err)
	}
	if err := (LegacyMainQueue{}).Remove(context.Background(), "root"); !errors.Is(err, ErrMainQueueOwner) || admissionQueuePending() != 0 {
		t.Fatal("foreign remove admission", err)
	}
	if err := q.RemovePath(canceled, "box", "/root"); !errors.Is(err, context.Canceled) || admissionQueuePending() != 0 {
		t.Fatal("canceled path admission", err)
	}
	if err := (LegacyMainQueue{}).RemovePath(context.Background(), "box", "/root"); !errors.Is(err, ErrMainQueueOwner) || admissionQueuePending() != 0 {
		t.Fatal("foreign path admission", err)
	}

	replacement := &parse.Tree{ID: "root", Root: &ast.Node{ID: "root", Type: ast.NodeDocument}}
	if err := q.UpdateRefs(context.Background(), tree); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateRefs(context.Background(), replacement); err != nil {
		t.Fatal(err)
	}
	if err := q.Remove(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	if err := q.Remove(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	if err := q.DeleteBox(context.Background(), "box"); err != nil {
		t.Fatal(err)
	}
	if err := q.DeleteBox(context.Background(), "box"); err != nil {
		t.Fatal(err)
	}
	if err := q.RemovePath(context.Background(), "box", "/root"); err != nil {
		t.Fatal(err)
	}
	if err := q.RemovePath(context.Background(), "box", "/root"); err != nil {
		t.Fatal(err)
	}
	dbQueueLock.Lock()
	if len(operationQueue) != 4 || operationQueue[0].ownerDB != owner || operationQueue[0].action != "update_refs" || operationQueue[0].upsertTree != replacement || operationQueue[1].action != "delete_id" || operationQueue[1].removeTreeID != "root" || operationQueue[2].action != "delete_box" || operationQueue[2].box != "box" || operationQueue[3].action != "delete" || operationQueue[3].removeTreeBox != "box" || operationQueue[3].removeTreePath != "/root" {
		t.Error("captured notebook admission/coalescing changed")
	}
	dbQueueLock.Unlock()

	db = &stdsql.DB{}
	if err := q.DeleteBox(context.Background(), "new-box"); !errors.Is(err, ErrMainQueueOwner) || admissionQueuePending() != 4 {
		t.Fatal("replacement database accepted stale notebook work", err)
	}
}

func TestMainQueueCapturedBlockContentAdmissionAndIsolation(t *testing.T) {
	owner := &stdsql.DB{}
	oldDB := db
	dbQueueLock.Lock()
	oldQueue := operationQueue
	operationQueue = nil
	dbQueueLock.Unlock()
	db = owner
	t.Cleanup(func() {
		dbQueueLock.Lock()
		operationQueue = oldQueue
		dbQueueLock.Unlock()
		db = oldDB
	})

	q := CaptureLegacyMainQueue()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.UpdateBlockContent(canceled, &Block{ID: "a"}); !errors.Is(err, context.Canceled) || admissionQueuePending() != 0 {
		t.Fatal("canceled content admission", err)
	}
	if err := q.UpdateBlockContent(context.Background(), nil); !errors.Is(err, ErrMainQueueOperation) || admissionQueuePending() != 0 {
		t.Fatal("invalid content admission", err)
	}
	if err := (LegacyMainQueue{}).UpdateBlockContent(context.Background(), &Block{ID: "a"}); !errors.Is(err, ErrMainQueueOwner) || admissionQueuePending() != 0 {
		t.Fatal("foreign content admission", err)
	}

	first := &Block{ID: "a", Content: "first"}
	second := &Block{ID: "a", Content: "second"}
	if err := q.UpdateBlockContent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateBlockContent(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	first.Content, second.Content = "changed", "changed"
	dbQueueLock.Lock()
	if len(operationQueue) != 1 || operationQueue[0].ownerDB != owner || operationQueue[0].action != "update_block_content" || operationQueue[0].block.Content != "second" {
		t.Error("captured content admission/coalescing changed")
	}
	dbQueueLock.Unlock()

	db = &stdsql.DB{}
	if err := q.UpdateBlockContent(context.Background(), &Block{ID: "b"}); !errors.Is(err, ErrMainQueueOwner) || admissionQueuePending() != 1 {
		t.Fatal("replacement database accepted stale content work", err)
	}
}
