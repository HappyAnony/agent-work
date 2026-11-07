package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOwnedDatabaseCacheConfigAndReferences(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{})
	ctx := context.Background()
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
	if a.workspace.resources.db == b.workspace.resources.db {
		t.Fatal("shared DB")
	}
	for i := 0; i < 3; i++ {
		if _, err := a.ReadBlock(ctx, boxID, docID, blockID); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := a.ResourceStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.IndexedDocuments != 1 || stats.IndexedBlocks != 4 || stats.IndexedReferences != 1 || stats.Parses != 1 {
		t.Fatal(stats)
	}
	other, err := b.ResourceStats(ctx)
	if err != nil || other.IndexedBlocks != 0 || other.CachedDocuments != 0 {
		t.Fatal(other, err)
	}
	refs, err := a.ReadReferences(ctx, boxID, docID)
	if err != nil || len(refs) != 1 || refs[0].TargetID != blockID || refs[0].SourceID != "20261019090004-ref0001" {
		t.Fatal(refs, err)
	}
	refs[0].TargetID = "tampered"
	refs, err = a.ReadReferences(ctx, boxID, docID)
	if err != nil || refs[0].TargetID != blockID {
		t.Fatal(refs, err)
	}
	conf, err := a.NotebookConfig(ctx, boxID)
	if err != nil || conf.Name != "Fixture notebook" {
		t.Fatal(conf, err)
	}
	conf.Raw[0] = 'x'
	conf, err = a.NotebookConfig(ctx, boxID)
	if err != nil || !json.Valid(conf.Raw) {
		t.Fatal(conf, err)
	}
	if digest(t, root) != before {
		t.Fatal("read/index initialized source files")
	}
	configFile := filepath.Join(root, "pages", pageB, "data", boxID, ".wiki", "conf.json")
	if err := os.WriteFile(configFile, []byte(`{"name":"B config","sort":3,"closed":true,"future":{"enabled":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	conf, err = b.NotebookConfig(ctx, boxID)
	if err != nil || conf.Name != "B config" || !bytes.Contains(conf.Raw, []byte(`"future"`)) {
		t.Fatal(conf, err)
	}
	conf, err = a.NotebookConfig(ctx, boxID)
	if err != nil || conf.Name != "Fixture notebook" || conf.Closed {
		t.Fatal("config crossed workspaces", conf, err)
	}
	if err := os.WriteFile(configFile, []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.NotebookConfig(ctx, boxID); err == nil {
		t.Fatal("accepted invalid config")
	}
	if _, err := a.ReadDocument(ctx, boxID, docID); err != nil {
		t.Fatal("bad config in B broke A", err)
	}
}

func TestHashRefreshUnknownASTAndFailedParseNoStaleSuccess(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReadBlock(ctx, boxID, docID, blockID); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(original, []byte("Synthetic knowledge"), []byte("Different knowledge"), 1)
	if err := os.WriteFile(file, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	block, err := l.ReadBlock(ctx, boxID, docID, blockID)
	if err != nil || !bytes.Contains(block, []byte("Different knowledge")) {
		t.Fatal(string(block), err)
	}
	// Unknown fields and property order must survive, including child IDs placed
	// after Children, which rules out relying on serialization field order.
	unknown := []byte(fmt.Sprintf(` { "Children":[{"Children":[{"Type":"FutureNode","opaque":{"x":[1,2]}}],"ID":%q,"Type":"FutureBlock"}],"ID":%q,"Spec":"1","unknown":{"a":true}} `, blockID, docID))
	if err := os.WriteFile(file, unknown, 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := l.ReadDocument(ctx, boxID, docID)
	if err != nil || !bytes.Equal(doc, unknown) {
		t.Fatal(string(doc), err)
	}
	block, err = l.ReadBlock(ctx, boxID, docID, blockID)
	if err != nil || !bytes.Contains(block, []byte(`"opaque"`)) || !json.Valid(block) {
		t.Fatal(string(block), err)
	}
	stats, err := l.ResourceStats(ctx)
	if err != nil || stats.Parses != 3 || stats.IndexedReferences != 0 {
		t.Fatal(stats, err)
	}
	if err := os.WriteFile(file, []byte(fmt.Sprintf(`{"ID":%q,"Children":[{"ID":%q},{"ID":%q}]}`, docID, blockID, blockID)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReadBlock(ctx, boxID, docID, blockID); err == nil {
		t.Fatal("returned stale cache for invalid source")
	}
	if err := os.WriteFile(file, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReadBlock(ctx, boxID, docID, blockID); err != nil {
		t.Fatal("parser did not recover", err)
	}
}

func TestCacheBudgetAndRealResourceClose(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("20261019090001-doc%04d", i)
		raw := []byte(fmt.Sprintf(`{"ID":%q,"Type":"NodeDocument"}`, id))
		file := filepath.Join(root, "pages", pageA, "data", boxID, id+".docj")
		if err := os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := l.ReadDocument(ctx, boxID, id); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := l.ResourceStats(ctx)
	if err != nil || stats.CachedDocuments != 8 || stats.IndexedDocuments != 8 || stats.IndexedBlocks != 8 {
		t.Fatal(stats, err)
	}
	r := l.workspace.resources
	dataRoot := l.workspace.data
	storeRoot := l.workspace.store.root
	l.Release()
	if err := m.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.db.PingContext(ctx); err == nil {
		t.Fatal("SQLite remained open")
	}
	if _, err := dataRoot.Stat("."); err == nil {
		t.Fatal("data handle remained open")
	}
	if _, err := storeRoot.Stat("."); err == nil {
		t.Fatal("store handle remained open")
	}
	if len(r.cache) != 0 || r.bytes != 0 {
		t.Fatal("mutable memory retained")
	}
}

func TestTaskOwnershipCancellationPanicAndDrain(t *testing.T) {
	m := open(t, fixture(t), Options{MaxTasksPerWorkspace: 1, MaxWorkspaces: 1})
	ctx := context.Background()
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	proceed := make(chan struct{})
	task, err := l.StartTask(ctx, func(taskCtx context.Context, lease *Lease) error {
		lease.Release()
		close(started)
		<-proceed
		return taskCtx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := l.StartTask(ctx, func(context.Context, *Lease) error { return nil }); !errors.Is(err, ErrBudget) {
		t.Fatal(err)
	}
	l.Release()
	if _, err := m.Acquire(ctx, storeID, pageB); !errors.Is(err, ErrBudget) {
		t.Fatal("early callback release unpinned task", err)
	}
	deadline, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err := m.Drain(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if m.Stats().Tasks != 1 {
		t.Fatal(m.Stats())
	}
	task.Cancel()
	close(proceed)
	if err := task.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := m.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if m.Stats().Tasks != 0 || m.Stats().Leases != 0 {
		t.Fatal(m.Stats())
	}

	m2 := open(t, fixture(t), Options{})
	l2, err := m2.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Release()
	panicked, err := l2.StartTask(ctx, func(context.Context, *Lease) error { panic("synthetic") })
	if err != nil {
		t.Fatal(err)
	}
	if err := panicked.Wait(ctx); err == nil || err.Error() != "workspace_task_panicked" {
		t.Fatal(err)
	}
	if _, err := l2.ReadDocument(ctx, boxID, docID); err != nil {
		t.Fatal("task panic destroyed context", err)
	}
}

func TestIndexQueueCancellationAndIndependentWorkspace(t *testing.T) {
	m := open(t, fixture(t), Options{})
	ctx := context.Background()
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
	<-a.workspace.resources.gate // Hold A's queue, not the manager or B's queue.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()
		if _, err := a.ReadBlock(deadline, boxID, docID, blockID); !errors.Is(err, context.DeadlineExceeded) {
			t.Error(err)
		}
	}()
	if _, err := b.ReadBlock(ctx, boxID, docID, blockID); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	a.workspace.resources.gate <- struct{}{}
	if _, err := a.ReadBlock(ctx, boxID, docID, blockID); err != nil {
		t.Fatal(err)
	}
}

func TestStoreOwnsWatcherAndWaitsForClose(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	if err := m.WatchStore(ctx, storeID); err != nil {
		t.Fatal(err)
	}
	if err := m.WatchStore(ctx, storeID); err != nil {
		t.Fatal(err)
	}
	if m.Stats().Watchers != 1 {
		t.Fatal(m.Stats())
	}
	watch := m.stores[storeID].watch
	metadata := filepath.Join(root, "pages", pageA, "page.json")
	data, err := os.ReadFile(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		l, err := m.Acquire(ctx, storeID, pageA)
		if errors.Is(err, ErrStale) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		l.Release()
		if time.Now().After(deadline) {
			t.Fatal("watch did not invalidate admission")
		}
		time.Sleep(time.Millisecond)
	}
	if err := m.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watch.Done():
	default:
		t.Fatal("Drain returned before watcher stopped")
	}
	if m.Stats().Watchers != 0 {
		t.Fatal(m.Stats())
	}
}

func TestDatabaseFailureDoesNotBreakOtherWorkspace(t *testing.T) {
	ctx := context.Background()
	m := open(t, fixture(t), Options{})
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
	if err := b.workspace.resources.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ReadBlock(ctx, boxID, docID, blockID); err == nil {
		t.Fatal("read succeeded on closed DB")
	}
	if _, err := a.ReadBlock(ctx, boxID, docID, blockID); err != nil {
		t.Fatal("B DB failure broke A", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if resources, err := openResources(canceled); err == nil || resources != nil {
		t.Fatal("opened resources after cancellation", err)
	}
}

func TestCancelledSQLiteTransactionKeepsSchemaAndFailedIndexIsNotPublished(t *testing.T) {
	ctx := context.Background()
	m := open(t, fixture(t), Options{})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	r := l.workspace.resources
	if err := r.serial(ctx, func() error {
		deadline, cancel := context.WithCancel(ctx)
		defer cancel()
		tx, err := r.db.BeginTx(deadline, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(deadline, "INSERT INTO documents VALUES('test','test',X'00')"); err != nil {
			return err
		}
		cancel()
		_ = tx.Rollback()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReadBlock(ctx, boxID, docID, blockID); err != nil {
		t.Fatal("cancelled transaction destroyed DB schema", err)
	}
	if _, err := r.db.ExecContext(ctx, `CREATE TRIGGER fail_index BEFORE INSERT ON blocks BEGIN SELECT RAISE(FAIL,'synthetic index failure'); END`); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(l.workspace.paths.DataDir, boxID, docID+".docj")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, bytes.ReplaceAll(raw, []byte("Synthetic knowledge"), []byte("Changed knowledge")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReadBlock(ctx, boxID, docID, blockID); err == nil {
		t.Fatal("claimed successful read after failed index")
	}
	stats, err := l.ResourceStats(ctx)
	if err != nil || stats.IndexedBlocks != 4 || stats.IndexedReferences != 1 {
		t.Fatal("failed index partially committed", stats, err)
	}
	if bytes.Contains(r.cache[documentKey{boxID, docID}].raw, []byte("Changed knowledge")) {
		t.Fatal("published cache before index commit")
	}
	if _, err := r.db.ExecContext(ctx, "DROP TRIGGER fail_index"); err != nil {
		t.Fatal(err)
	}
	block, err := l.ReadBlock(ctx, boxID, docID, blockID)
	if err != nil || !bytes.Contains(block, []byte("Changed knowledge")) {
		t.Fatal(string(block), err)
	}
}
