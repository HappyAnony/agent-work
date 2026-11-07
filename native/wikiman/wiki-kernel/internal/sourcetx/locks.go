// Package sourcetx coordinates portable-source transactions. Callers must never
// write page.json/docj directly or remove stable lock files to claim ownership.
package sourcetx

import (
	"context"
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"wiki-kernel/internal/wikistore"
)

var (
	ErrBusy     = errors.New("store_busy")
	ErrReadOnly = errors.New("source_read_only")
	ErrRecovery = errors.New("recovery_required")
	ErrConflict = errors.New("source_conflict")
	ErrInvalid  = errors.New("invalid_transaction")
	ErrClosed   = errors.New("source_session_closed")
	ErrIndex    = errors.New("index_stale")
	ErrIdentity = errors.New("source_identity_changed")
)

type Store struct {
	Root         string
	identityHash string
	mu           sync.Mutex
	epoch        string
	generation   uint64
	tree         *wikistore.TreeSnapshot
	events       []Event
}

// Open provisions runtime-only lock/journal directories, never source records.
// First access to an unprovisioned read-only root fails closed, not lockless.
func Open(root string) (*Store, error) {
	return openSource(root, true)
}

// OpenInspection does not create runtime directories or lock files.
func OpenInspection(root string) (*Store, error) {
	return openSource(root, false)
}

func openSource(root string, provision bool) (*Store, error) {
	l, err := wikistore.OpenLayout(root)
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(l.Root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	identity, exists, err := readFile(r, "store.json", 1<<20)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, os.ErrNotExist
	}
	var store wikistore.Store
	if err := strict(identity, &store); err != nil {
		return nil, ErrInvalid
	}
	storeRef := &Store{Root: l.Root, identityHash: Hash(identity), epoch: rand.Text()}
	if !provision {
		return storeRef, nil
	}
	for _, name := range []string{".local", ".local/locks", ".local/journal", ".local/staging", ".local/receipts", ".local/index"} {
		if err := safePath(r, name); err != nil {
			return nil, err
		}
		if err := r.MkdirAll(name, 0700); err != nil {
			return nil, writeError(err)
		}
		if err := syncDir(r, name); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"session.lock", "source.lock"} {
		name = ".local/locks/" + name
		if err := safePath(r, name); err != nil {
			return nil, err
		}
		if info, err := r.Lstat(name); err == nil && !info.Mode().IsRegular() {
			return nil, ErrInvalid
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, writeError(err)
		}
		f, err := r.OpenFile(name, os.O_CREATE|os.O_RDONLY, 0600)
		if err != nil {
			return nil, writeError(err)
		}
		if err = f.Close(); err != nil {
			return nil, err
		}
	}
	if err := syncDir(r, ".local"); err != nil {
		return nil, err
	}
	if err := syncDir(r, "."); err != nil {
		return nil, err
	}
	return storeRef, nil
}

func (s *Store) checkIdentity(root *os.Root) error {
	raw, exists, err := readFile(root, "store.json", 1<<20)
	if err != nil {
		return err
	}
	if !exists || Hash(raw) != s.identityHash {
		return ErrIdentity
	}
	return nil
}

func safePath(root *os.Root, name string) error {
	if name == "" || strings.Contains(name, "\\") || strings.ContainsRune(name, 0) || !fs.ValidPath(name) {
		return ErrInvalid
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		if i < len(parts)-1 && !info.IsDir() {
			return ErrInvalid
		}
	}
	return nil
}

func writeError(err error) error {
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS) {
		return errors.Join(ErrReadOnly, err)
	}
	return err
}

func (s *Store) lock(ctx context.Context, name string, shared, wait bool) (*flock.Flock, error) {
	r, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	name = ".local/locks/" + name + ".lock"
	if err := safePath(r, name); err != nil {
		return nil, err
	}
	if info, err := r.Lstat(name); err != nil || !info.Mode().IsRegular() {
		return nil, errors.Join(ErrInvalid, err)
	}
	l := flock.New(filepath.Join(s.Root, filepath.FromSlash(name)), flock.SetFlag(os.O_RDONLY))
	var ok bool
	if wait {
		if shared {
			ok, err = l.TryRLockContext(ctx, 10*time.Millisecond)
		} else {
			ok, err = l.TryLockContext(ctx, 10*time.Millisecond)
		}
	} else {
		if err = ctx.Err(); err == nil {
			if shared {
				ok, err = l.TryRLock()
			} else {
				ok, err = l.TryLock()
			}
		}
	}
	if err != nil {
		_ = l.Close()
		return nil, err
	}
	if !ok {
		_ = l.Close()
		return nil, ErrBusy
	}
	return l, nil
}

// Read holds one shared source lock for the entire callback. It refuses an
// unfinished journal before exposing any source bytes (including tree metadata).
func (s *Store) Read(ctx context.Context, fn func() error) error {
	return s.ReadContext(ctx, func(context.Context) error { return fn() })
}

type readAdmissionKey struct{}
type readAdmission struct {
	store   *Store
	mu      sync.Mutex
	cond    *sync.Cond
	active  bool
	readers int
}

// ReadContext validates once while holding the shared source lock. Only child
// reads of this exact Store may borrow it. Closing first revokes new borrowers,
// then drains admitted readers before unlocking; a retained context fails closed.
func (s *Store) ReadContext(ctx context.Context, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a, ok := ctx.Value(readAdmissionKey{}).(*readAdmission); ok {
		if a.store != s {
			return ErrIdentity
		}
		a.mu.Lock()
		if !a.active {
			a.mu.Unlock()
			return ErrClosed
		}
		a.readers++
		a.mu.Unlock()
		defer func() { a.mu.Lock(); a.readers--; a.cond.Broadcast(); a.mu.Unlock() }()
		if err := ctx.Err(); err != nil {
			return err
		}
		return fn(ctx)
	}
	l, err := s.lock(ctx, "source", true, true)
	if err != nil {
		return err
	}
	defer l.Close()
	r, err := os.OpenRoot(s.Root)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := s.checkIdentity(r); err != nil {
		return err
	}
	if err := admit(r); err != nil {
		return err
	}
	a := &readAdmission{store: s, active: true}
	a.cond = sync.NewCond(&a.mu)
	defer func() {
		a.mu.Lock()
		a.active = false
		for a.readers > 0 {
			a.cond.Wait()
		}
		a.mu.Unlock()
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(context.WithValue(ctx, readAdmissionKey{}, a))
}

type Writer struct {
	store   *Store
	session *flock.Flock
	queue   chan struct{}
	closed  bool
}

// Writer owns session → source lock order; operations serialize under mu. GUI
// may keep it until release. Local CLI owns it for one operation then closes.
func (s *Store) Writer(ctx context.Context, wait bool) (*Writer, error) {
	l, err := s.lock(ctx, "session", false, wait)
	if err != nil {
		return nil, err
	}
	w := &Writer{store: s, session: l, queue: make(chan struct{}, 1)}
	w.queue <- struct{}{}
	return w, nil
}

func (w *Writer) Close() error {
	return w.CloseContext(context.Background())
}

func (w *Writer) CloseContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.queue:
	}
	defer func() { w.queue <- struct{}{} }()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.session.Close()
}

func (w *Writer) exclusive(ctx context.Context, allowRecovery bool, fn func(*os.Root) error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.queue:
	}
	defer func() { w.queue <- struct{}{} }()
	if w.closed {
		return ErrClosed
	}
	l, err := w.store.lock(ctx, "source", false, true)
	if err != nil {
		return err
	}
	defer l.Close()
	r, err := os.OpenRoot(w.store.Root)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := w.store.checkIdentity(r); err != nil {
		return err
	}
	if !allowRecovery {
		if err := admit(r); err != nil {
			return err
		}
	}
	return fn(r)
}
