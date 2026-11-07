package workspace

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/internal/sourcetx"
)

// Source mutations require releasing live leases first, as with Apply. Returning
// a source receipt here is legitimate: this uses the same journal/CAS coordinator.
func (m *WorkspaceManager) SaveWorkspacePreferences(ctx context.Context, store, page, operation, expected string, doc conf.PreferenceDocument) (sourcetx.Receipt, error) {
	request, err := sourcetx.WorkspacePreferencesRequest(page, operation, expected, doc)
	if err != nil {
		return sourcetx.Receipt{}, err
	}
	return m.Apply(ctx, store, request)
}

func (m *WorkspaceManager) SaveDevicePreferences(ctx context.Context, store, expected string, doc conf.PreferenceDocument) (hash string, err error) {
	if err = doc.Validate(conf.DevicePreference); err != nil {
		return "", err
	}
	doc, err = doc.Clone()
	if err != nil {
		return "", err
	}
	err = m.mutate(ctx, store, false, func(w *sourcetx.Writer) error {
		var err error
		hash, err = w.SaveDevicePreferences(ctx, expected, doc)
		return err
	})
	return
}

func (m *WorkspaceManager) DevicePreferences(ctx context.Context, store string) (doc conf.PreferenceDocument, hash string, err error) {
	err = m.withStoreReadContext(ctx, store, func(c context.Context, s *StoreContext) error {
		var err error
		doc, hash, err = s.source.DevicePreferences(c)
		return err
	})
	return
}

// Shared preferences invalidate every page Context through the same store-wide
// mutation barrier as source edits. No device secrets or runtime paths enter it.
func (m *WorkspaceManager) SaveStorePreferences(ctx context.Context, store, operation, expected string, doc conf.PreferenceDocument) (sourcetx.Receipt, error) {
	request, err := sourcetx.StorePreferencesRequest(operation, expected, doc)
	if err != nil {
		return sourcetx.Receipt{}, err
	}
	return m.Apply(ctx, store, request)
}
