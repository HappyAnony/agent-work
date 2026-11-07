// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path"
	"strings"
)

type sourceCopyEntry struct {
	name string
	info fs.FileInfo
}
type sourceCopyIO struct {
	read func(context.Context, Source, string) ([]byte, error)
}

// CopyDirectoryFrom preserves the original merge and sequential success-prefix
// contract. Complete enumeration precedes destination writes; each source entry
// is rechecked and each destination file uses the existing revision comparison.
func (s Source) CopyDirectoryFrom(ctx context.Context, input Source, from, to string) error {
	return s.copyDirectory(ctx, input, from, to, sourceCopyIO{})
}
func (s Source) copyDirectory(ctx context.Context, input Source, from, to string, op sourceCopyIO) error {
	if ctx == nil || input.Data == nil || s.Data == nil || !fs.ValidPath(from) || !fs.ValidPath(to) || strings.ContainsAny(from+to, "\\") {
		return ErrSourcePath
	}
	if e := s.write(ctx); e != nil {
		return e
	}
	entries := []sourceCopyEntry{}
	check := func(saved sourceCopyEntry) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if saved.name != "." {
			if e := input.Check(saved.name, false); e != nil {
				return e
			}
		}
		info, e := input.Data.Lstat(saved.name)
		if e != nil {
			return e
		}
		if info.Mode()&fs.ModeSymlink != 0 || !os.SameFile(info, saved.info) || info.Mode() != saved.info.Mode() || info.Size() != saved.info.Size() || !info.ModTime().Equal(saved.info.ModTime()) {
			return ErrSourceChanged
		}
		return nil
	}
	var scan func(string, int) error
	scan = func(name string, depth int) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if depth > 255 {
			return ErrSourcePath
		}
		if name != "." {
			if e := input.Check(name, false); e != nil {
				return e
			}
		}
		info, e := input.Data.Lstat(name)
		if e != nil {
			return e
		}
		if info.Mode()&fs.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return ErrSourcePath
		}
		entries = append(entries, sourceCopyEntry{name, info})
		if info.IsDir() {
			children, e := input.ReadDir(ctx, name)
			if e != nil {
				return e
			}
			for _, child := range children {
				if e = scan(path.Join(name, child.Name()), depth+1); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e := scan(from, 0); e != nil {
		return e
	}
	if !entries[0].info.IsDir() {
		return ErrSourcePath
	}
	for _, entry := range entries {
		if e := check(entry); e != nil {
			return e
		}
	}
	if op.read == nil {
		op.read = func(c context.Context, s Source, name string) ([]byte, error) { return s.Read(c, name) }
	}
	for _, entry := range entries {
		if e := s.write(ctx); e != nil {
			return e
		}
		if e := check(entry); e != nil {
			return e
		}
		relative := entry.name
		if from != "." {
			relative = strings.TrimPrefix(entry.name, from)
			relative = strings.TrimPrefix(relative, "/")
		}
		target := path.Join(to, relative)
		if target == "." {
			if !entry.info.IsDir() {
				return ErrSourcePath
			}
			continue
		}
		if e := s.Check(target, true); e != nil {
			return e
		}
		if entry.info.IsDir() {
			if e := s.Data.MkdirAll(target, 0755); e != nil {
				return e
			}
			if e := s.Check(target, false); e != nil {
				return e
			}
			continue
		}
		raw, e := op.read(ctx, input, entry.name)
		if e != nil {
			return e
		}
		if e = check(entry); e != nil {
			return e
		}
		before, e := s.Read(ctx, target)
		if e != nil && !IsSourceNotExist(e) {
			return e
		}
		if e == nil && bytes.Equal(before, raw) {
			continue
		}
		if e = s.Rewrite(ctx, target, before, raw); e != nil {
			return e
		}
	}
	for _, entry := range entries {
		if e := check(entry); e != nil {
			return e
		}
	}
	return ctx.Err()
}
