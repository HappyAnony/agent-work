// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"strings"
)

var ErrSourcePath = errors.New("invalid_repair_source_path")
var ErrSourceWrite = errors.New("repair_source_write_unbound")
var ErrSourceChanged = errors.New("repair_source_changed")

// Roots are borrowed for the enclosing operation. Authorize is a separate
// trusted compatibility capability, never implied by possession of a reader.
// Lock may bridge the original in-process file lock; cross-process CAS/journal
// and a multi-file transaction are intentionally not provided here.
type Source struct {
	Data, ArchiveRoot *os.Root
	Authorize         func(context.Context) error
	Lock              func(string) func()
}

func (s Source) Check(name string, missing bool) error {
	if s.Data == nil || !fs.ValidPath(name) || name == "." || strings.Contains(name, "\\") {
		return ErrSourcePath
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, e := s.Data.Lstat(strings.Join(parts[:i+1], "/"))
		if missing && errors.Is(e, fs.ErrNotExist) {
			return nil
		}
		if e != nil {
			return e
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return ErrSourcePath
		}
		if i < len(parts)-1 && !info.IsDir() {
			return ErrSourcePath
		}
	}
	return nil
}
func (s Source) Read(ctx context.Context, name string) ([]byte, error) {
	return s.read(ctx, name, sourceReadIO{})
}
func (s Source) write(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if s.Authorize == nil {
		return ErrSourceWrite
	}
	return s.Authorize(ctx)
}
func nonce() (string, error) {
	var b [12]byte
	_, e := rand.Read(b[:])
	return hex.EncodeToString(b[:]), e
}
func (s Source) locked(name string) func() {
	if s.Lock != nil {
		return s.Lock(name)
	}
	return func() {}
}

// Rewrite uses an atomic same-directory publication. nil before requires a new
// path; non-nil requires exact original bytes. Once temporary output begins,
// finish publication/cleanup before honoring a later request cancellation.
func (s Source) Rewrite(ctx context.Context, name string, before, after []byte) error {
	return s.rewrite(ctx, name, before, after, sourceRewriteIO{})
}

// Remove/rename are explicit source mutations; neither follows symlinks.
func (s Source) Remove(ctx context.Context, name string) error {
	return s.remove(ctx, name, sourceMutationIO{})
}

// RemoveUnchanged checks the backup revision under the same mutation lock.
// It preserves the original single-file successful-prefix publication boundary.
func (s Source) RemoveUnchanged(ctx context.Context, name string, before []byte) error {
	return s.remove(ctx, name, sourceMutationIO{compare: true, expected: before})
}
func (s Source) EnsureDirectory(ctx context.Context, name string) error {
	if e := s.write(ctx); e != nil {
		return e
	}
	if e := s.Check(name, true); e != nil {
		return e
	}
	if e := s.Data.MkdirAll(name, 0755); e != nil {
		return e
	}
	if e := s.Check(name, false); e != nil {
		return e
	}
	return s.write(ctx)
}
func (s Source) Rename(ctx context.Context, from, to string) error {
	return s.rename(ctx, from, to, sourceMutationIO{})
}
func (s Source) RemoveLegacyHistory(ctx context.Context, box string) error {
	return s.removeLegacyHistory(ctx, box, sourceLifecycleIO{})
}
func (s Source) Archive(ctx context.Context, name, stamp string) (string, error) {
	return s.archive(ctx, name, stamp, sourceLifecycleIO{})
}
