// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

type sourceReadDirFile interface {
	ReadDir(int) ([]fs.DirEntry, error)
	Stat() (fs.FileInfo, error)
	Close() error
}
type sourceReadDirIO struct {
	open      func(*os.Root, string) (sourceReadDirFile, error)
	closeRoot func(*os.Root) error
}

func (op sourceReadDirIO) defaults() sourceReadDirIO {
	if op.open == nil {
		op.open = func(root *os.Root, name string) (sourceReadDirFile, error) { return root.Open(name) }
	}
	if op.closeRoot == nil {
		op.closeRoot = func(root *os.Root) error { return root.Close() }
	}
	return op
}

// ReadDir borrows its outer Source root and needs no write authority or lock.
// Entries retain fs.ReadDir's lexical order. Selected-read failures, including
// cleanup failures, are never classified as pure initial missing directories.
func (s Source) ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error) {
	return s.readDir(ctx, name, sourceReadDirIO{})
}
func (s Source) readDir(ctx context.Context, name string, op sourceReadDirIO) (out []fs.DirEntry, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if s.Data == nil {
		return nil, ErrSourcePath
	}
	if name != "." {
		if err = s.Check(name, false); err != nil {
			return nil, err
		}
	}
	op = op.defaults()
	var directory *sourceDirectory
	var input sourceReadDirFile
	var expected fs.FileInfo
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if directory != nil {
			if err := directory.check(); err != nil {
				return err
			}
		}
		if expected == nil {
			return nil
		}
		now, err := s.Data.Lstat(name)
		if err != nil {
			return err
		}
		if !now.IsDir() || now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(now, expected) || now.Size() != expected.Size() || !now.ModTime().Equal(expected.ModTime()) {
			return ErrSourceChanged
		}
		return nil
	}
	defer func() {
		if input != nil {
			err = errors.Join(err, input.Close())
		}
		err = errors.Join(err, check())
		if directory != nil {
			err = errors.Join(err, op.closeRoot(directory.root), check())
		}
		if err != nil {
			out = nil
			err = errors.Join(ErrSourceReadIncomplete, err)
		}
	}()
	expected, err = s.Data.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !expected.IsDir() {
		return nil, ErrSourcePath
	}
	directory, err = borrowSourceDirectory(s, name, sourceLifecycleIO{closeRoot: op.closeRoot})
	if err != nil {
		return nil, err
	}
	if !os.SameFile(expected, directory.identity) {
		return nil, ErrSourceChanged
	}
	if err = check(); err != nil {
		return nil, err
	}
	input, err = op.open(directory.root, ".")
	if err != nil {
		return nil, err
	}
	verify := func() error {
		if err := check(); err != nil {
			return err
		}
		info, err := input.Stat()
		if err != nil {
			return err
		}
		if !info.IsDir() || !os.SameFile(info, expected) || info.Size() != expected.Size() || !info.ModTime().Equal(expected.ModTime()) {
			return ErrSourceChanged
		}
		return nil
	}
	seen := map[string]bool{}
	out = make([]fs.DirEntry, 0)
	for {
		if err = verify(); err != nil {
			return nil, err
		}
		entries, readErr := input.ReadDir(128)
		if err = verify(); err != nil {
			if readErr == io.EOF {
				readErr = nil
			}
			return nil, errors.Join(readErr, err)
		}
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		for _, entry := range entries {
			n := entry.Name()
			if !fs.ValidPath(n) || n == "." || path.Base(n) != n || seen[n] {
				return nil, ErrSourcePath
			}
			seen[n] = true
		}
		out = append(out, entries...)
		if readErr == io.EOF {
			break
		}
		if len(entries) == 0 {
			return nil, io.ErrNoProgress
		}
	}
	slices.SortFunc(out, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return out, check()
}
