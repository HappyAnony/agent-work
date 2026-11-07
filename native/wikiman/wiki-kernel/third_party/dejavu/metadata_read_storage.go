// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"context"
	"io/fs"
	"os"
)

// MetadataReadStorage is bound before a request-owned repository is shared.
// Its creator owns the root, context, identity checks and final close barrier.
type MetadataReadStorage struct {
	ReadFile func(string) ([]byte, error)
	ReadDir  func(string) ([]fs.DirEntry, error)
	Stat     func(string) (fs.FileInfo, error)
}

func (repo *Repo) BindMetadataReadStorage(ctx context.Context, p MetadataReadStorage) error {
	if ctx == nil || repo.store == nil || p.ReadFile == nil || p.ReadDir == nil || p.Stat == nil {
		return ErrSnapshot
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	copy := p
	repo.store.readStorage = &copy
	repo.store.operation = ctx
	repo.store.scoped = true
	return nil
}
func (s *Store) readMetadataFile(name string) ([]byte, error) {
	if e := s.check(); e != nil {
		return nil, e
	}
	if s.readStorage != nil {
		return s.readStorage.ReadFile(name)
	}
	return os.ReadFile(name)
}
func (s *Store) readMetadataDirectory(name string) ([]fs.DirEntry, error) {
	if e := s.check(); e != nil {
		return nil, e
	}
	if s.readStorage != nil {
		return s.readStorage.ReadDir(name)
	}
	return os.ReadDir(name)
}
func (s *Store) statMetadataFile(name string) (fs.FileInfo, error) {
	if e := s.check(); e != nil {
		return nil, e
	}
	if s.readStorage != nil {
		return s.readStorage.Stat(name)
	}
	return os.Stat(name)
}
