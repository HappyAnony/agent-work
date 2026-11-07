package model

import (
	"context"
	"errors"
	"github.com/siyuan-note/filelock"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type ownedResourceFiles struct {
	owner *legacyReindexOwner
	root  *os.Root
}

func (f ownedResourceFiles) read(ctx context.Context, name string) ([]byte, error) {
	if e := f.owner.guard(ctx); e != nil {
		return nil, e
	}
	file, e := f.root.Open(name)
	if e != nil {
		return nil, e
	}
	raw, e := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	e = errors.Join(e, file.Close())
	if e != nil {
		return nil, e
	}
	if len(raw) > 16<<20 {
		return nil, errors.New("owned_resource_budget")
	}
	return raw, ctx.Err()
}
func (f ownedResourceFiles) write(ctx context.Context, name string, raw []byte) error {
	if e := f.owner.guard(ctx); e != nil {
		return e
	}
	c := f.owner.owner
	c.m.Lock()
	ro := c.persistenceReadOnly
	c.m.Unlock()
	if ro {
		return ErrSettingReadOnly
	}
	p := filepath.Join(f.owner.data, name)
	filelock.Lock(p)
	defer filelock.Unlock(p)
	if e := f.owner.guard(ctx); e != nil {
		return e
	}
	if e := f.root.MkdirAll(filepath.Dir(name), 0755); e != nil {
		return e
	}
	file, e := f.root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	_, e = file.Write(raw)
	return errors.Join(e, file.Close())
}
func (f ownedResourceFiles) readDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	if e := f.owner.guard(ctx); e != nil {
		return nil, e
	}
	entries, e := fs.ReadDir(f.root.FS(), filepath.ToSlash(name))
	if e != nil {
		return nil, e
	}
	return entries, ctx.Err()
}
