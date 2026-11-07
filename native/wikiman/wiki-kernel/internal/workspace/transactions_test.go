package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikistore"
)

func renameRequest(t *testing.T, root, op string) sourcetx.Request {
	t.Helper()
	name := "pages/" + pageA + "/page.json"
	raw, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	var p wikistore.Page
	if json.Unmarshal(raw, &p) != nil {
		t.Fatal("bad fixture")
	}
	p.Name = "Renamed by transaction"
	p.Revision++
	next, _ := json.Marshal(p)
	return sourcetx.Request{OperationID: op, Changes: []sourcetx.Change{{Path: name, ExpectedHash: sourcetx.Hash(raw), Data: next}}}
}

func TestAdmittedNestedReadsFinishWhileOtherPageRequestsWaitForMutation(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered, proceed := make(chan struct{}), make(chan struct{})
	readDone := make(chan error, 1)
	var retained context.Context
	go func() {
		readDone <- m.WithReadContext(ctx, storeID, func(scoped context.Context) error {
			retained = scoped
			close(entered)
			<-proceed
			if _, err := m.Tree(scoped, storeID); err != nil {
				return err
			}
			if _, err := m.SourceFile(scoped, storeID, "pages/"+pageA+"/data/"+boxID+"/"+docID+".docj"); err != nil {
				return err
			}
			if _, _, err := m.DocumentSource(scoped, storeID, pageA, boxID, docID); err != nil {
				return err
			}
			lease, err := m.Acquire(scoped, storeID, pageA)
			if err != nil {
				return err
			}
			defer lease.Release()
			_, err = lease.ReadDocument(scoped, boxID, docID)
			return err
		})
	}()
	<-entered
	mutationDone := make(chan error, 1)
	request := renameRequest(t, root, "nested-read-admission")
	go func() { _, err := m.Apply(ctx, storeID, request); mutationDone <- err }()
	for {
		m.mu.Lock()
		mutating := m.stores[storeID].mutating
		m.mu.Unlock()
		if mutating {
			break
		}
		if ctx.Err() != nil {
			close(proceed)
			t.Fatal("mutation never entered gate")
		}
		time.Sleep(time.Millisecond)
	}
	otherRead := make(chan error, 1)
	go func() { _, err := m.Tree(ctx, storeID); otherRead <- err }()
	select {
	case err := <-otherRead:
		close(proceed)
		t.Fatal("new read failed instead of waiting for publication", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(proceed)
	if err := <-readDone; err != nil {
		t.Fatal("admitted nested read was rejected", err)
	}
	if err := <-mutationDone; err != nil {
		t.Fatal(err)
	}
	if err := <-otherRead; err != nil {
		t.Fatal("new read did not resume", err)
	}
	if _, err := m.Tree(retained, storeID); !errors.Is(err, ErrClosed) {
		t.Fatal("retained read scope reopened source", err)
	}
	if lease, err := m.Acquire(retained, storeID, pageA); !errors.Is(err, ErrClosed) {
		if lease != nil {
			lease.Release()
		}
		t.Fatal("retained scope acquired workspace", err)
	}
}

func TestStoreReadRequestsWithoutWorkspaceLeasesDrainBeforeMutationAndClose(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	if err := m.ClaimSession(ctx, storeID, false); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- m.WithRead(ctx, storeID, func() error { close(entered); <-release; return nil }) }()
	<-entered
	deadline, cancel := context.WithTimeout(ctx, 25*time.Millisecond)
	if _, err := m.Apply(deadline, storeID, renameRequest(t, root, "read-request-gate")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("mutation did not wait for direct source read", err)
	}
	cancel()
	deadline, cancel = context.WithTimeout(ctx, 25*time.Millisecond)
	if err := m.DrainStore(deadline, storeID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("drain closed an active logical read", err)
	}
	cancel()
	source, err := sourcetx.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if writer, err := source.Writer(ctx, false); err == nil {
		writer.Close()
		t.Fatal("drain released session while a source read was active")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := m.DrainStore(ctx, storeID); err != nil {
		t.Fatal(err)
	}
	writer, err := source.Writer(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	writer.Close()
}

func TestRecoveryRequiredIsNotReportedAsAClosedWorkspace(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	m.mu.Lock()
	m.stores[storeID].recoveryRequired = true
	m.mu.Unlock()
	if _, err := m.Tree(ctx, storeID); !errors.Is(err, sourcetx.ErrRecovery) {
		t.Fatal(err)
	}
	if _, err := m.Rebuild(ctx, storeID, "all"); !errors.Is(err, sourcetx.ErrRecovery) {
		t.Fatal(err)
	}
	if err := m.ClaimSession(ctx, storeID, false); !errors.Is(err, sourcetx.ErrRecovery) {
		t.Fatal(err)
	}
	if _, err := m.Recover(ctx, storeID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Tree(ctx, storeID); err != nil {
		t.Fatal("recovery did not lift gate", err)
	}
}

func TestStoreMutationGateDrainsLeasesAndPublishesFreshContexts(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	if err := m.WatchStore(ctx, storeID); err != nil {
		t.Fatal(err)
	}
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	oldGeneration := l.Generation()
	deadline, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := m.Apply(deadline, storeID, renameRequest(t, root, "blocked")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if _, err := l.ReadBlock(ctx, boxID, docID, blockID); err != nil {
		t.Fatal("timed-out writer broke existing lease", err)
	}
	l.Release()
	r, err := m.Apply(ctx, storeID, renameRequest(t, root, "gated-rename"))
	if err != nil || !r.SourceCommitted || !r.IndexFresh {
		t.Fatal(r, err)
	}
	for i := 0; i < 4; i++ {
		next, err := m.Acquire(ctx, storeID, pageA)
		if err != nil {
			t.Fatal("controlled write left dirty watcher/admission", err)
		}
		if next.Generation() == oldGeneration {
			t.Fatal("write reused old generation")
		}
		p, _ := next.workspace.store.tree.Get(pageA)
		if p.Name != "Renamed by transaction" {
			t.Fatal("old tree remained published", p)
		}
		if _, err := next.ReadBlock(ctx, boxID, docID, blockID); err != nil {
			t.Fatal(err)
		}
		next.Release()
		time.Sleep(time.Millisecond)
	}
}

func TestPendingJournalRejectsExistingReadsAndKeepsMutationFrozen(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	source, err := sourcetx.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := source.Writer(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = writer.Apply(ctx, renameRequest(t, root, "unfinished"), func(point string) error {
		if point == "prepared" {
			return errors.New("interrupt")
		}
		return nil
	})
	if err == nil {
		t.Fatal("did not interrupt")
	}
	writer.Close()
	if _, err := l.ReadDocument(ctx, boxID, docID); !errors.Is(err, sourcetx.ErrRecovery) {
		t.Fatal("existing workspace bypassed journal", err)
	}
	l.Release()
	if _, err := m.Apply(ctx, storeID, renameRequest(t, root, "must-not-write")); !errors.Is(err, sourcetx.ErrRecovery) {
		t.Fatal(err)
	}
	if _, err := m.Acquire(ctx, storeID, pageA); !errors.Is(err, ErrClosed) {
		t.Fatal("failed recovery admission unfroze store", err)
	}
}

func TestOwnedEditorSessionIsBorrowedAndReleasedOnlyAfterDrain(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	if err := m.ClaimSession(ctx, storeID, false); err != nil {
		t.Fatal(err)
	}
	other, err := sourcetx.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	assertBusy := func() {
		t.Helper()
		if writer, err := other.Writer(ctx, false); !errors.Is(err, sourcetx.ErrBusy) {
			if writer != nil {
				writer.Close()
			}
			t.Fatal("editor session not retained", err)
		}
	}
	assertBusy()
	if r, err := m.Apply(ctx, storeID, renameRequest(t, root, "borrow-owned-session")); err != nil || !r.IndexFresh {
		t.Fatal("reacquired own session instead of borrowing", r, err)
	}
	if _, err := m.Rebuild(ctx, storeID, "all"); err != nil {
		t.Fatal(err)
	}
	assertBusy()
	// An interrupted operation owned by the editor is explicitly recovered via
	// that same owner; another process never steals or unlocks its session.
	writer := m.stores[storeID].writer
	_, err = writer.Apply(ctx, renameRequest(t, root, "owner-interrupted"), func(point string) error {
		if point == "prepared" {
			return errors.New("interrupt")
		}
		return nil
	})
	if err == nil {
		t.Fatal("did not interrupt")
	}
	if _, err := m.Apply(ctx, storeID, renameRequest(t, root, "must-freeze")); !errors.Is(err, sourcetx.ErrRecovery) {
		t.Fatal(err)
	}
	if _, err := m.Acquire(ctx, storeID, pageA); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := m.Recover(ctx, storeID); err != nil {
		t.Fatal("could not recover using owned session", err)
	}
	assertBusy()
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	if err := m.DrainStore(ctx, storeID); err != nil {
		t.Fatal(err)
	}
	released, err := other.Writer(ctx, false)
	if err != nil {
		t.Fatal("drain leaked writer session", err)
	}
	released.Close()
}

func TestBodyOnlyCreationInvalidatesEmptyPinnedDataRoot(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	empty := "1760000000000000004"
	l, err := m.Acquire(ctx, storeID, empty)
	if err != nil {
		t.Fatal(err)
	}
	if l.workspace.data != nil {
		t.Fatal("fixture page is not empty")
	}
	source, err := sourcetx.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := source.Writer(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := writer.Apply(ctx, sourcetx.Request{OperationID: "body-only-created", Changes: []sourcetx.Change{{Path: "pages/" + empty + "/data/" + boxID + "/" + docID + ".docj", Data: raw}}}, nil)
	writer.Close()
	if err != nil || !r.IndexFresh {
		t.Fatal(r, err)
	}
	if _, err := l.ReadDocument(ctx, boxID, docID); !errors.Is(err, ErrStale) {
		t.Fatal("stale empty handle masqueraded as current missing document", err)
	}
	l.Release()
	if _, err := m.EvictIdle(time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	next, err := m.Acquire(ctx, storeID, empty)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	if _, err := next.ReadDocument(ctx, boxID, docID); err != nil {
		t.Fatal(err)
	}
}

func TestStoreWriteQueueSerializesAndCancelsWaitingRequests(t *testing.T) {
	root := fixture(t)
	m := open(t, root, Options{})
	ctx := context.Background()
	lease, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	firstReq := renameRequest(t, root, "queue-first")
	secondReq := renameRequest(t, root, "queue-second")
	first := make(chan error, 1)
	go func() { _, err := m.Apply(ctx, storeID, firstReq); first <- err }()
	end := time.Now().Add(time.Second)
	for {
		m.mu.Lock()
		active := m.stores[storeID].mutating
		m.mu.Unlock()
		if active {
			break
		}
		if time.Now().After(end) {
			t.Fatal("writer never entered gate")
		}
		time.Sleep(time.Millisecond)
	}
	deadline, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := m.Apply(deadline, storeID, secondReq); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("waiting request was rejected instead of cancellably queued", err)
	}
	second := make(chan error, 1)
	go func() { _, err := m.Apply(ctx, storeID, secondReq); second <- err }()
	select {
	case err := <-second:
		t.Fatal("second write bypassed queue", err)
	case <-time.After(10 * time.Millisecond):
	}
	lease.Release()
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; !errors.Is(err, sourcetx.ErrConflict) {
		t.Fatal("queued CAS was not rechecked against committed source", err)
	}
}
