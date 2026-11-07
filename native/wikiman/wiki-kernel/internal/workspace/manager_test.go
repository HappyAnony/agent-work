package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const storeID = "f36f1884-2299-4fde-88c1-9da8f26d2aac"
const pageA = "1760000000000000002"
const pageB = "1760000000000000003"
const boxID = "20261019090000-box0001"
const docID = "20261019090001-doc0001"
const blockID = "20261019090002-par0001"

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "pages", pageA, "data")
	b := filepath.Join(root, "pages", pageB, "data")
	if err := os.MkdirAll(b, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(b, os.DirFS(a)); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(b, boxID, docID+".docj")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, bytes.ReplaceAll(data, []byte("Synthetic knowledge"), []byte("Workspace B knowledge")), 0600); err != nil {
		t.Fatal(err)
	}
	return root
}

func digest(t *testing.T, root string) [32]byte {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(root, name)
		if relative == ".local" && d.IsDir() {
			return fs.SkipDir
		}
		h.Write([]byte(relative))
		if !d.IsDir() {
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			h.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

func open(t *testing.T, root string, options Options) *WorkspaceManager {
	t.Helper()
	m := NewManager(options)
	if id, err := m.OpenStore(context.Background(), root); err != nil || id != storeID {
		t.Fatalf("open: %s %v", id, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := m.Drain(ctx); err != nil {
			t.Error(err)
		}
	})
	return m
}

func TestConcurrentIsolationAndInitialization(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	generations := map[string]uint64{}
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			page := pageA
			expected := "Synthetic knowledge"
			if i%2 != 0 {
				page = pageB
				expected = "Workspace B knowledge"
			}
			l, err := m.Acquire(context.Background(), storeID, page)
			if err != nil {
				t.Error(err)
				return
			}
			defer l.Release()
			mu.Lock()
			if old := generations[page]; old != 0 && old != l.Generation() {
				t.Error("duplicate initialization")
			}
			generations[page] = l.Generation()
			mu.Unlock()
			data, err := l.ReadBlock(context.Background(), boxID, docID, blockID)
			if err != nil || !bytes.Contains(data, []byte(expected)) {
				t.Errorf("%s crossed workspace: %s %v", page, data, err)
			}
			if len(data) > 0 {
				data[0] = 'x'
			} // Detached result cannot corrupt another request.
		}(i)
	}
	wg.Wait()
	if m.Stats().Workspaces != 2 || m.Stats().Leases != 0 {
		t.Fatal(m.Stats())
	}
	if generations[pageA] == generations[pageB] {
		t.Fatal("generation collision")
	}
	if after := digest(t, root); after != before {
		t.Fatal("source modified by reads")
	}
}

func TestBudgetTasksEvictionAndStaleLease(t *testing.T) {
	m := open(t, fixture(t), Options{MaxWorkspaces: 1, IdleTimeout: time.Second})
	ctx := context.Background()
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	task, err := l.Retain()
	if err != nil {
		t.Fatal(err)
	}
	generation := l.Generation()
	l.Release()
	l.Release()
	if _, err := l.ReadDocument(ctx, boxID, docID); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if _, err := m.Acquire(ctx, storeID, pageB); !errors.Is(err, ErrBudget) {
		t.Fatal(err)
	}
	if n, err := m.EvictIdle(time.Now().Add(time.Hour)); err != nil || n != 0 {
		t.Fatalf("evicted task %d %v", n, err)
	}
	if _, err := task.ReadDocument(ctx, boxID, docID); err != nil {
		t.Fatal(err)
	}
	task.Release()
	if n, err := m.EvictIdle(time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("idle close %d %v", n, err)
	}
	newLease, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer newLease.Release()
	if newLease.Generation() == generation {
		t.Fatal("stale generation reused")
	}
}

func TestIdentityFailureIsolationAndAliases(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{})
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenStore(ctx, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := m.OpenStore(ctx, fixture(t)); !errors.Is(err, ErrAmbiguous) {
		t.Fatal(err)
	}
	for _, ids := range [][2]string{{"", pageA}, {storeID, ""}, {"unknown", pageA}, {storeID, "../../bad"}} {
		if _, err := m.Acquire(ctx, ids[0], ids[1]); !errors.Is(err, ErrIdentity) {
			t.Fatal(err)
		}
	}
	bad := filepath.Join(root, "pages", "1760000000000000004", "data")
	if err := os.WriteFile(bad, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Acquire(ctx, storeID, "1760000000000000004"); err == nil {
		t.Fatal("accepted invalid workspace")
	}
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if _, err := l.ReadDocument(ctx, boxID, docID); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReadDocument(ctx, "../outside", docID); err == nil {
		t.Fatal("accepted traversal")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := m.Acquire(canceled, storeID, pageA); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDrainTimeoutDoesNotForceClose(t *testing.T) {
	m := open(t, fixture(t), Options{})
	ctx := context.Background()
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err := m.Drain(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if m.Stats().Leases != 1 || m.Stats().Workspaces != 1 {
		t.Fatal(m.Stats())
	}
	if _, err := m.Acquire(ctx, storeID, pageB); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := l.Retain(); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err := l.ReadDocument(ctx, boxID, docID); err != nil {
		t.Fatal("drain prevented existing request from completing", err)
	}
	l.Release()
	if err := m.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if m.Stats().Stores != 0 || m.Stats().Workspaces != 0 {
		t.Fatal(m.Stats())
	}
}

func TestSamePageIdentityInDistinctStores(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	other := fixture(t)
	newID := "a36f1884-2299-4fde-88c1-9da8f26d2aac"
	metadata := filepath.Join(other, "store.json")
	raw, err := os.ReadFile(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadata, bytes.ReplaceAll(raw, []byte(storeID), []byte(newID)), 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(other, "pages", pageA, "data", boxID, docID+".docj")
	raw, err = os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, bytes.ReplaceAll(raw, []byte("Synthetic knowledge"), []byte("Other store knowledge")), 0600); err != nil {
		t.Fatal(err)
	}
	m := open(t, root, Options{})
	if _, err := m.OpenStore(ctx, other); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{storeID, newID, storeID, newID} {
		l, err := m.Acquire(ctx, id, pageA)
		if err != nil {
			t.Fatal(err)
		}
		data, err := l.ReadBlock(ctx, boxID, docID, blockID)
		l.Release()
		if err != nil {
			t.Fatal(err)
		}
		expected := "Synthetic knowledge"
		if id == newID {
			expected = "Other store knowledge"
		}
		if !bytes.Contains(data, []byte(expected)) {
			t.Fatal("cross-store read", string(data))
		}
	}
	a, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err := m.DrainStore(deadline, storeID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if _, err := m.Acquire(ctx, storeID, pageB); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	b, err := m.Acquire(ctx, newID, pageA)
	if err != nil {
		t.Fatal("draining A stopped B", err)
	}
	if _, err := b.ReadBlock(ctx, boxID, docID, blockID); err != nil {
		t.Fatal(err)
	}
	b.Release()
	oldGeneration := a.Generation()
	a.Release()
	if err := m.DrainStore(ctx, storeID); err != nil {
		t.Fatal(err)
	}
	if m.Stats().Stores != 1 {
		t.Fatal(m.Stats())
	}
	if _, err := m.OpenStore(ctx, root); err != nil {
		t.Fatal(err)
	}
	reopened, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Release()
	if reopened.Generation() == oldGeneration {
		t.Fatal("reopened source reused generation")
	}
}

func TestMalformedDocumentAndSymlinkAreRejected(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	m := open(t, root, Options{})
	l, err := m.Acquire(ctx, storeID, pageA)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	file := filepath.Join(root, "pages", pageA, "data", boxID, docID+".docj")
	if err := os.WriteFile(file, []byte(`{"ID":"wrong"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReadDocument(ctx, boxID, docID); err == nil {
		t.Fatal("accepted invalid identity")
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "pages", pageB, "data", boxID, docID+".docj"), file); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReadDocument(ctx, boxID, docID); err == nil {
		t.Fatal("followed cross-page symlink")
	}
}
