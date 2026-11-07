// SPDX-License-Identifier: AGPL-3.0-or-later
package notebooks

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

type notebookDirectory interface {
	ReadDir(int) ([]fs.DirEntry, error)
	Stat() (fs.FileInfo, error)
	Close() error
}
type notebookWriter interface {
	Write([]byte) (int, error)
	Close() error
}

// Trusted, invocation-local ports; no caller-supplied repair authority is added.
type notebookIO struct {
	read          func(context.Context, *os.Root, string) ([]byte, error)
	openDirectory func(*os.Root) (notebookDirectory, error)
	create        func(*os.Root, string) (notebookWriter, error)
}

func (op notebookIO) defaults() notebookIO {
	if op.read == nil {
		op.read = read
	}
	if op.openDirectory == nil {
		op.openDirectory = func(r *os.Root) (notebookDirectory, error) { return r.Open(".") }
	}
	if op.create == nil {
		op.create = func(r *os.Root, n string) (notebookWriter, error) {
			return r.OpenFile(n, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		}
	}
	return op
}

func notebookEntries(ctx context.Context, root *os.Root, op notebookIO) (out []fs.DirEntry, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	before, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	input, err := op.openDirectory(root)
	if err != nil {
		return nil, err
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		now, err := root.Stat(".")
		if err != nil {
			return err
		}
		if !now.IsDir() || !os.SameFile(before, now) || now.Size() != before.Size() || !now.ModTime().Equal(before.ModTime()) {
			return ErrChanged
		}
		return nil
	}
	defer func() {
		err = errors.Join(err, input.Close(), check())
		if err != nil {
			out = nil
		}
	}()
	verify := func() error {
		if err := check(); err != nil {
			return err
		}
		now, err := input.Stat()
		if err != nil {
			return err
		}
		if !now.IsDir() || !os.SameFile(before, now) || now.Size() != before.Size() || !now.ModTime().Equal(before.ModTime()) {
			return ErrChanged
		}
		return nil
	}
	seen := map[string]bool{}
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
		if len(out)+len(entries) > 100000 {
			return nil, ErrPath
		}
		for _, entry := range entries {
			n := entry.Name()
			if !fs.ValidPath(n) || n == "." || path.Base(n) != n || strings.Contains(n, "\\") || seen[n] {
				return nil, ErrPath
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
	return out, nil
}
