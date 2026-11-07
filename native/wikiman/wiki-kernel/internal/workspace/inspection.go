package workspace

import (
	"context"

	"wiki-kernel/internal/sourcetx"
)

func (m *WorkspaceManager) inspectionSource(id string) (*sourcetx.Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	store := m.stores[id]
	if store == nil {
		return nil, ErrIdentity
	}
	if m.draining || store.draining {
		return nil, ErrClosed
	}
	return store.source, nil
}

func (m *WorkspaceManager) Inspect(ctx context.Context, id string) (sourcetx.Inspection, error) {
	source, err := m.inspectionSource(id)
	if err != nil {
		return sourcetx.Inspection{}, err
	}
	return source.Inspect(ctx, id)
}

func (m *WorkspaceManager) PreviewRecovery(ctx context.Context, id string) ([]sourcetx.RecoveryStep, error) {
	source, err := m.inspectionSource(id)
	if err != nil {
		return nil, err
	}
	return source.PreviewRecovery(ctx, id)
}
