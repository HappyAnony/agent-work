// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package announcement

import (
	"context"
	"errors"
	"fmt"
	"github.com/gofrs/flock"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// FileStore keeps the legacy device-only location/array format. Construction
// does no IO; this root is trusted device authority, never a request/wiki path.
type FileStore struct {
	home string
	gate chan struct{}
}

const ledgerPath = ".config/siyuan/announcement.json"
const ledgerDir = ".config/siyuan"

func NewFileStore(home string) (*FileStore, error) {
	if !filepath.IsAbs(home) {
		return nil, ErrDependency
	}
	return &FileStore{home: filepath.Clean(home), gate: make(chan struct{}, 1)}, nil
}
func check(root *os.Root, name string) (bool, error) {
	for _, p := range []string{".config", ledgerDir, name} {
		v, e := root.Lstat(p)
		if errors.Is(e, fs.ErrNotExist) {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		if v.Mode()&fs.ModeSymlink != 0 {
			return false, ErrLedger
		}
		if p != name && !v.IsDir() || p == name && !v.Mode().IsRegular() {
			return false, ErrLedger
		}
	}
	return true, nil
}
func readLedger(root *os.Root) ([]byte, bool, error) {
	exists, e := check(root, ledgerPath)
	if e != nil || !exists {
		return nil, false, e
	}
	f, e := root.Open(ledgerPath)
	if e != nil {
		return nil, false, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	if len(b) > 16<<20 {
		return nil, false, ErrLedger
	}
	return b, true, e
}
func (f *FileStore) Update(ctx context.Context, merge func([]byte, bool) ([]byte, bool, error)) error {
	if merge == nil {
		return ErrDependency
	}
	select {
	case f.gate <- struct{}{}:
		defer func() { <-f.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	r, e := os.OpenRoot(f.home)
	if e != nil {
		return e
	}
	defer r.Close()
	raw, exists, e := readLedger(r)
	if e != nil {
		return e
	}
	_, write, e := merge(raw, exists)
	if e != nil || !write {
		return e
	}
	for _, dir := range []string{".config", ledgerDir} {
		if e = r.Mkdir(dir, 0700); e != nil && !errors.Is(e, fs.ErrExist) {
			return e
		}
		v, e := r.Lstat(dir)
		if e != nil {
			return e
		}
		if !v.IsDir() || v.Mode()&fs.ModeSymlink != 0 {
			return ErrLedger
		}
	}
	lockPath := ledgerDir + "/.announcement.lock"
	if _, e = check(r, lockPath); e != nil {
		return e
	}
	lock := flock.New(filepath.Join(f.home, lockPath))
	ok, e := lock.TryLockContext(ctx, 10*time.Millisecond)
	if e != nil {
		lock.Close()
		return e
	}
	if !ok {
		lock.Close()
		return ctx.Err()
	}
	defer func() { lock.Unlock(); lock.Close() }()
	raw, exists, e = readLedger(r)
	if e != nil {
		return e
	}
	next, write, e := merge(raw, exists)
	if e != nil || !write {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	// A private exclusive temporary file avoids truncating the current ledger.
	var temp *os.File
	var name string
	for i := 0; i < 8; i++ {
		name = fmt.Sprintf("%s/.announcement-%d-%d.tmp", ledgerDir, time.Now().UnixNano(), i)
		temp, e = r.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e == nil {
			break
		}
		if !errors.Is(e, fs.ErrExist) {
			return e
		}
	}
	if e != nil {
		return e
	}
	defer r.Remove(name)
	_, e = temp.Write(next)
	if e == nil {
		e = temp.Sync()
	}
	e = errors.Join(e, temp.Close())
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return r.Rename(name, ledgerPath)
}
