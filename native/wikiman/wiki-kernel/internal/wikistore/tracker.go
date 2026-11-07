package wikistore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

var ErrTreeDirty = errors.New("tree snapshot requires refresh")

// Tracker invalidates snapshots; events never directly create/delete nodes.
// Refresh intentionally rescans even if no event arrived, so event loss and Git
// timestamp restoration cannot make an old index appear authoritative.
type Tracker struct {
	root      string
	scanner   Scanner
	refreshMu sync.Mutex
	mu        sync.RWMutex
	epoch     uint64
	dirty     bool
	snapshot  *TreeSnapshot
	lastError error
}

func NewTracker(root string, scanner Scanner) *Tracker {
	return &Tracker{root: root, scanner: scanner, dirty: true}
}

func (t *Tracker) Invalidate() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.epoch++
	t.dirty = true
}

func (t *Tracker) View() (*TreeSnapshot, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.lastError != nil {
		return nil, t.lastError
	}
	if t.dirty || t.snapshot == nil {
		return nil, ErrTreeDirty
	}
	return t.snapshot, nil
}

func (t *Tracker) Refresh(ctx context.Context) (*TreeSnapshot, error) {
	t.refreshMu.Lock()
	defer t.refreshMu.Unlock()
	for attempt := 0; attempt < 3; attempt++ {
		t.mu.RLock()
		epoch := t.epoch
		t.mu.RUnlock()
		next, err := t.scanner.Scan(ctx, t.root)
		t.mu.Lock()
		if err != nil {
			t.lastError = err
			t.dirty = true
			t.mu.Unlock()
			return nil, err
		}
		if epoch != t.epoch {
			t.mu.Unlock()
			continue
		}
		if t.snapshot != nil && t.snapshot.SourceDigest() == next.SourceDigest() && t.snapshot.StoreDigest() == next.StoreDigest() {
			next = t.snapshot
		}
		t.snapshot = next
		t.lastError = nil
		t.dirty = false
		t.mu.Unlock()
		return next, nil
	}
	err := &ScanError{Type: "source_unstable", Diagnostics: []Diagnostic{{Code: "source_unstable", Severity: "error", RelativePath: "pages"}}}
	t.mu.Lock()
	t.lastError = err
	t.dirty = true
	t.mu.Unlock()
	return nil, err
}

type Watch struct {
	watcher *fsnotify.Watcher
	once    sync.Once
	done    chan struct{}
}

func (w *Watch) stop()                 { w.once.Do(func() { _ = w.watcher.Close() }) }
func (w *Watch) Close()                { w.stop(); <-w.done }
func (w *Watch) Done() <-chan struct{} { return w.done }

// StartWatch is optional and explicit. It only watches root/page metadata
// directories, and never starts legacy cron, recursively scans bodies, or
// creates a .local directory. StoreContext owns its final lifecycle in S03.
func (t *Tracker) StartWatch(ctx context.Context) (*Watch, error) {
	layout, err := OpenLayout(t.root)
	if err != nil {
		return nil, err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watch{watcher: watcher, done: make(chan struct{})}
	if err := watcher.Add(layout.Root); err != nil {
		w.stop()
		return nil, err
	}
	resubscribe := func() error {
		if err := watcher.Add(layout.PagesDir()); err != nil {
			return err
		}
		entries, err := os.ReadDir(layout.PagesDir())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && pageIDPattern.MatchString(entry.Name()) {
				if err := watcher.Add(filepath.Join(layout.PagesDir(), entry.Name())); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := resubscribe(); err != nil {
		w.stop()
		return nil, err
	}
	go func() {
		defer close(w.done)
		defer w.stop()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				rel, err := filepath.Rel(layout.Root, event.Name)
				if err != nil {
					t.Invalidate()
					continue
				}
				rel = filepath.ToSlash(rel)
				if rel == "store.json" || rel == "pages" || strings.HasPrefix(rel, "pages/") {
					t.Invalidate()
					if event.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
						_ = resubscribe()
					}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
				t.Invalidate()
			}
		}
	}()
	return w, nil
}
