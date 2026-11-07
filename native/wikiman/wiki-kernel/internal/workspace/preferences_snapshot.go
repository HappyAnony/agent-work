package workspace

import (
	"context"
	"time"
	"wiki-kernel/internal/sourcetx"
)

// Read-only B1 root-preference component. Capture under WithReadContext to
// compose with other committed-source reads using one shared source admission.
func (m *WorkspaceManager) SnapshotStorePreferences(ctx context.Context, id string, at time.Time) (out sourcetx.StorePreferencesSnapshot, err error) {
	err = m.withStoreReadContext(ctx, id, func(scoped context.Context, s *StoreContext) error {
		var e error
		out, e = s.source.SnapshotStorePreferences(scoped, at)
		return e
	})
	return
}
func (m *WorkspaceManager) PlanRestoreStorePreferences(ctx context.Context, id string, v sourcetx.StorePreferencesSnapshot, operation, expected string) (out sourcetx.StorePreferencesRestorePlan, err error) {
	err = m.withStoreReadContext(ctx, id, func(scoped context.Context, s *StoreContext) error {
		var e error
		out, e = s.source.PlanRestoreStorePreferences(scoped, v, operation, expected)
		return e
	})
	return
}

// Release every live lease before restoring. The existing store-wide mutation
// barrier drains requests/resources and rebuilds all page Contexts, including
// pages affected solely by shared preference changes and missing-file fallback.
func (m *WorkspaceManager) RestoreStorePreferences(ctx context.Context, id string, p sourcetx.StorePreferencesRestorePlan) (out sourcetx.Receipt, err error) {
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if a, ok := ctx.Value(readAdmissionKey{}).(*readAdmission); ok {
		if a.manager != m {
			return out, sourcetx.ErrIdentity
		}
		m.mu.Lock()
		active := m.admitted(ctx, m.stores[id])
		m.mu.Unlock()
		if !active {
			return out, ErrClosed
		}
		return out, sourcetx.ErrBusy
	}
	if e := p.Validate(); e != nil {
		return out, e
	}
	if id != p.Snapshot.Store.StoreID {
		return out, sourcetx.ErrIdentity
	}
	// Freeze caller-owned bytes before waiting for leases/source locks.
	p.Snapshot.Data = append([]byte(nil), p.Snapshot.Data...)
	err = m.mutate(ctx, id, false, func(w *sourcetx.Writer) error { var e error; out, e = w.RestoreStorePreferences(ctx, p, nil); return e })
	return
}
