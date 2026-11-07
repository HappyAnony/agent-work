package workspace

import (
	"context"
	"errors"
	"os"

	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikistore"
)

func (l *Lease) sourceRead(ctx context.Context, fn func() error) error {
	s := l.workspace.store
	return s.source.Read(ctx, func() error {
		tree, err := (wikistore.Scanner{}).Scan(ctx, s.layout.Root)
		if err != nil {
			return err
		}
		if tree.SourceDigest() != s.tree.SourceDigest() || tree.StoreDigest() != s.tree.StoreDigest() {
			return ErrStale
		}
		// A body-only change can create/replace a data directory without changing
		// page.json. Never read an old pinned directory as today's source.
		info, err := s.root.Lstat("pages/" + l.PageID() + "/data")
		if errors.Is(err, os.ErrNotExist) {
			if l.workspace.data != nil {
				return ErrStale
			}
		} else if err != nil {
			return err
		} else {
			if !info.IsDir() || l.workspace.data == nil {
				return ErrStale
			}
			pinned, err := l.workspace.data.Stat(".")
			if err != nil {
				return err
			}
			if !os.SameFile(info, pinned) {
				return ErrStale
			}
		}
		return fn()
	})
}

// WithRead covers the entire logical request, including configuration + AST.
// Per-lease reads also guard direct callers; each lock owns its descriptor.
func (m *WorkspaceManager) WithRead(ctx context.Context, id string, fn func() error) error {
	return m.withStoreRead(ctx, id, func(_ *StoreContext) error { return fn() })
}

type readAdmissionKey struct{}
type readAdmission struct {
	manager *WorkspaceManager
	store   *StoreContext
	active  bool
}

// WithReadContext pins the full logical request. Nested reads/Acquire carry the
// same admission even while a mutation/drain waits for this request to finish.
// A context retained past callback return is invalid, never a fresh admission.
func (m *WorkspaceManager) WithReadContext(ctx context.Context, id string, fn func(context.Context) error) error {
	return m.withStoreReadContext(ctx, id, func(scoped context.Context, s *StoreContext) error {
		admission := &readAdmission{manager: m, store: s, active: true}
		defer func() { m.mu.Lock(); admission.active = false; m.mu.Unlock() }()
		return fn(context.WithValue(scoped, readAdmissionKey{}, admission))
	})
}

// m.mu must be held. The exact registration, not only a reusable store ID, binds
// nested requests to the source admitted before a drain/re-registration.
func (m *WorkspaceManager) admitted(ctx context.Context, s *StoreContext) bool {
	a, ok := ctx.Value(readAdmissionKey{}).(*readAdmission)
	return ok && a.active && a.manager == m && a.store == s && s != nil
}

func (m *WorkspaceManager) withStoreRead(ctx context.Context, id string, fn func(*StoreContext) error) error {
	return m.withStoreReadContext(ctx, id, func(_ context.Context, s *StoreContext) error { return fn(s) })
}

func (m *WorkspaceManager) withStoreReadContext(ctx context.Context, id string, fn func(context.Context, *StoreContext) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	s := m.stores[id]
	if admission, ok := ctx.Value(readAdmissionKey{}).(*readAdmission); ok && admission.manager == m {
		valid := m.admitted(ctx, s)
		m.mu.Unlock()
		if !valid {
			return ErrClosed
		}
		return fn(ctx, s)
	}
	// A short-lived mutation is not a closed workspace. Wait for its publication
	// barrier; true draining remains fail-closed for new admissions.
	for s != nil && s.mutating && !s.draining && !m.draining {
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
		m.mu.Lock()
		if m.stores[id] != s {
			m.mu.Unlock()
			return ErrClosed
		}
	}
	closed := m.draining || (s != nil && (s.draining || s.mutating))
	recovery := s != nil && s.recoveryRequired
	if s != nil && !closed && !recovery {
		s.requests++
	}
	m.mu.Unlock()
	if s == nil {
		return ErrIdentity
	}
	if closed {
		return ErrClosed
	}
	if recovery {
		return sourcetx.ErrRecovery
	}
	defer func() { m.mu.Lock(); s.requests--; close(m.changed); m.changed = make(chan struct{}); m.mu.Unlock() }()
	return s.source.ReadContext(ctx, func(scoped context.Context) error { return fn(scoped, s) })
}

// Apply gates and drains this store's leases, closes its read resources, then
// uses its source/session coordinator. Pending journals keep editing frozen.
func (m *WorkspaceManager) Apply(ctx context.Context, id string, request sourcetx.Request) (sourcetx.Receipt, error) {
	var result sourcetx.Receipt
	err := m.mutate(ctx, id, false, func(w *sourcetx.Writer) error { var err error; result, err = w.Apply(ctx, request, nil); return err })
	return result, err
}

func (m *WorkspaceManager) Preview(ctx context.Context, id string, request sourcetx.Request) (sourcetx.Receipt, error) {
	var result sourcetx.Receipt
	err := m.mutate(ctx, id, false, func(w *sourcetx.Writer) error { var err error; result, err = w.Preview(ctx, request); return err })
	return result, err
}

// Tree returns a newly validated immutable source snapshot, not a stale cache.
func (m *WorkspaceManager) Tree(ctx context.Context, id string) (*wikistore.TreeSnapshot, error) {
	var tree *wikistore.TreeSnapshot
	err := m.withStoreRead(ctx, id, func(s *StoreContext) error {
		var err error
		tree, err = s.source.ScanTree(ctx)
		if err != nil {
			return err
		}
		m.mu.Lock()
		registeredDigest := s.tree.StoreDigest()
		m.mu.Unlock()
		if tree.Store().StoreID != id || tree.StoreDigest() != registeredDigest {
			return ErrStale
		}
		return nil
	})
	return tree, err
}

// SourceFile is bounded and read-only; callers never receive an os.Root handle.
func (m *WorkspaceManager) SourceFile(ctx context.Context, id, name string) ([]byte, error) {
	var data []byte
	err := m.withStoreRead(ctx, id, func(s *StoreContext) error { var err error; data, err = s.source.SourceFile(ctx, name); return err })
	return data, err
}

func (m *WorkspaceManager) DocumentSource(ctx context.Context, id, page, box, doc string) (string, []byte, error) {
	var name string
	var data []byte
	err := m.withStoreRead(ctx, id, func(s *StoreContext) error {
		var err error
		name, data, err = s.source.Document(ctx, page, box, doc)
		return err
	})
	return name, data, err
}

func (m *WorkspaceManager) Rebuild(ctx context.Context, id, scope string) (sourcetx.IndexState, error) {
	var result sourcetx.IndexState
	err := m.mutate(ctx, id, false, func(w *sourcetx.Writer) error { var err error; result, err = w.Rebuild(ctx, scope); return err })
	return result, err
}

func (m *WorkspaceManager) Recover(ctx context.Context, id string) ([]sourcetx.Receipt, error) {
	var result []sourcetx.Receipt
	err := m.mutate(ctx, id, true, func(w *sourcetx.Writer) error { var err error; result, err = w.Recover(ctx, nil); return err })
	return result, err
}

func (m *WorkspaceManager) mutate(ctx context.Context, id string, recovery bool, fn func(*sourcetx.Writer) error) error {
	m.mu.Lock()
	s := m.stores[id]
	m.mu.Unlock()
	if s == nil {
		return ErrIdentity
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.writeQueue:
	}
	defer func() { s.writeQueue <- struct{}{} }()
	m.mu.Lock()
	if m.stores[id] != s {
		m.mu.Unlock()
		return ErrClosed
	}
	if m.draining || s.draining || s.mutating {
		m.mu.Unlock()
		return ErrClosed
	}
	if s.recoveryRequired && !recovery {
		m.mu.Unlock()
		return sourcetx.ErrRecovery
	}
	s.mutating = true
	for {
		active := s.requests > 0
		for _, w := range m.workspaces {
			if w.store == s && w.refs > 0 {
				active = true
				break
			}
		}
		if !active {
			break
		}
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			m.mu.Lock()
			s.mutating = false
			close(m.changed)
			m.changed = make(chan struct{})
			m.mu.Unlock()
			return ctx.Err()
		case <-changed:
		}
		m.mu.Lock()
	}
	var closeErr error
	watchCtx := s.watchCtx
	if s.watch != nil {
		s.watch.Close()
		s.watch = nil
	}
	for k, w := range m.workspaces {
		if w.store == s {
			closeErr = errors.Join(closeErr, m.closeWorkspace(k, w))
		}
	}
	ownedWriter := s.writer
	m.mu.Unlock()
	var applyErr error
	if closeErr != nil {
		applyErr = closeErr
	} else {
		writer := ownedWriter
		var err error
		if writer == nil {
			writer, err = s.source.Writer(ctx, false)
		}
		if err != nil {
			applyErr = err
		} else {
			applyErr = fn(writer)
			if ownedWriter == nil {
				applyErr = errors.Join(applyErr, writer.Close())
			}
		}
	}
	var next *wikistore.TreeSnapshot
	refreshErr := s.source.Read(ctx, func() error { var err error; next, err = s.tracker.Refresh(ctx); return err })
	m.mu.Lock()
	s.mutating = false
	if refreshErr != nil {
		s.recoveryRequired = true
	} else {
		s.recoveryRequired = false
		s.tree = next
		if watchCtx != nil && watchCtx.Err() == nil && !s.draining && !m.draining {
			s.watch, refreshErr = s.tracker.StartWatch(watchCtx)
		}
	}
	close(m.changed)
	m.changed = make(chan struct{})
	m.mu.Unlock()
	return errors.Join(applyErr, refreshErr)
}

// ClaimSession retains the store's writer across editor operations. Apply,
// Recover and Rebuild borrow it rather than reacquiring its own OS session lock.
// DrainStore/Drain release it only after requests and mutations have drained.
func (m *WorkspaceManager) ClaimSession(ctx context.Context, id string, wait bool) error {
	m.mu.Lock()
	s := m.stores[id]
	m.mu.Unlock()
	if s == nil {
		return ErrIdentity
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.writeQueue:
	}
	defer func() { s.writeQueue <- struct{}{} }()
	m.mu.Lock()
	if m.stores[id] != s {
		m.mu.Unlock()
		return ErrClosed
	}
	if m.draining || s.draining || s.mutating {
		m.mu.Unlock()
		return ErrClosed
	}
	if s.recoveryRequired {
		m.mu.Unlock()
		return sourcetx.ErrRecovery
	}
	if s.writer != nil {
		m.mu.Unlock()
		return nil
	}
	s.mutating = true
	m.mu.Unlock()
	writer, err := s.source.Writer(ctx, wait)
	m.mu.Lock()
	s.mutating = false
	if err == nil {
		if m.draining || s.draining {
			err = errors.Join(ErrClosed, writer.Close())
		} else {
			s.writer = writer
		}
	}
	close(m.changed)
	m.changed = make(chan struct{})
	m.mu.Unlock()
	return err
}
