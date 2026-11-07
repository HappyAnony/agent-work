package workspace

import (
	"context"
	"wiki-kernel/internal/sourcetx"
)

func (m *WorkspaceManager) HistoryChanges(ctx context.Context, id, operation, page string, redo bool, scope string) (out []sourcetx.Change, err error) {
	err = m.withStoreRead(ctx, id, func(s *StoreContext) error {
		out, err = s.source.HistoryChanges(ctx, operation, page, redo, scope)
		return err
	})
	return
}

func (m *WorkspaceManager) CommittedHashes(ctx context.Context, id, operation, page string) (out map[string]string, err error) {
	err = m.withStoreRead(ctx, id, func(s *StoreContext) error { out, err = s.source.CommittedHashes(ctx, operation, page); return err })
	return
}

func (m *WorkspaceManager) SourceFiles(ctx context.Context, id, page string) ([]sourcetx.FileInfo, error) {
	var files []sourcetx.FileInfo
	err := m.withStoreRead(ctx, id, func(s *StoreContext) error { var err error; files, err = s.source.Files(ctx, page); return err })
	return files, err
}
func (m *WorkspaceManager) LookupReceipt(ctx context.Context, id, op, hash string) (sourcetx.Receipt, bool, error) {
	var receipt sourcetx.Receipt
	var found bool
	err := m.withStoreRead(ctx, id, func(s *StoreContext) error {
		var err error
		receipt, found, err = s.source.Lookup(ctx, op, hash)
		return err
	})
	return receipt, found, err
}
func (m *WorkspaceManager) Events(ctx context.Context, id string) ([]sourcetx.Event, error) {
	var events []sourcetx.Event
	err := m.withStoreRead(ctx, id, func(s *StoreContext) error { events = s.source.Events(); return nil })
	return events, err
}

func (m *WorkspaceManager) PageReadGuards(ctx context.Context, id, page string) (files []sourcetx.FileInfo, guards *sourcetx.ReadGuards, err error) {
	err = m.withStoreRead(ctx, id, func(s *StoreContext) error {
		files, guards, err = s.source.PageReadGuards(ctx, page)
		return err
	})
	return
}

func (m *WorkspaceManager) ArchiveFiles(ctx context.Context, id, namespace, page, box, doc string) (files []sourcetx.FileInfo, err error) {
	err = m.withStoreRead(ctx, id, func(s *StoreContext) error {
		files, err = s.source.ArchiveFiles(ctx, namespace, page, box, doc)
		return err
	})
	return
}

func (m *WorkspaceManager) ArchiveReadGuards(ctx context.Context, id, namespace, page string) (files []sourcetx.FileInfo, guards *sourcetx.ReadGuards, err error) {
	err = m.withStoreRead(ctx, id, func(s *StoreContext) error {
		files, guards, err = s.source.ArchiveReadGuards(ctx, namespace, page)
		return err
	})
	return
}
