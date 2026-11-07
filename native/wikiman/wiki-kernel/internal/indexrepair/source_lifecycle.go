// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
)

type sourceLifecycleFile interface {
	io.Reader
	ReadDir(int) ([]fs.DirEntry, error)
	Stat() (fs.FileInfo, error)
	Sync() error
	Close() error
}

// Private operation ports are captured locally; public Source capabilities and
// production filesystem roots remain unchanged.
type sourceLifecycleIO struct {
	open      func(*os.Root, string) (sourceLifecycleFile, error)
	remove    func(*os.Root, string) error
	closeRoot func(*os.Root) error
	rewrite   sourceRewriteIO
}

func (op sourceLifecycleIO) defaults() sourceLifecycleIO {
	if op.open == nil {
		op.open = func(root *os.Root, name string) (sourceLifecycleFile, error) { return root.Open(name) }
	}
	if op.remove == nil {
		op.remove = func(root *os.Root, name string) error { return root.Remove(name) }
	}
	if op.closeRoot == nil {
		op.closeRoot = func(root *os.Root) error { return root.Close() }
	}
	return op
}

type sourceDirectory struct {
	source   Source
	name     string
	root     *os.Root
	identity fs.FileInfo
}

func borrowSourceDirectory(s Source, name string, op sourceLifecycleIO) (out *sourceDirectory, err error) {
	if name != "." {
		if err = s.Check(name, false); err != nil {
			return nil, err
		}
	}
	info, err := s.Data.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrSourcePath
	}
	root, err := s.Data.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err == nil && !os.SameFile(info, opened) {
		err = ErrSourceChanged
	}
	if err != nil {
		return nil, errors.Join(err, op.closeRoot(root))
	}
	return &sourceDirectory{source: s, name: name, root: root, identity: opened}, nil
}
func (d *sourceDirectory) check() error {
	if d.name != "." {
		if err := d.source.Check(d.name, false); err != nil {
			return err
		}
	}
	now, err := d.source.Data.Lstat(d.name)
	if err != nil {
		return err
	}
	if !now.IsDir() || !os.SameFile(now, d.identity) {
		return ErrSourceChanged
	}
	return nil
}
func sourceEntryMatches(root *os.Root, name string, expected fs.FileInfo) error {
	now, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if expected == nil || now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(now, expected) || now.Mode().Type() != expected.Mode().Type() {
		return ErrSourceChanged
	}
	if now.Mode().IsRegular() && (now.Size() != expected.Size() || !now.ModTime().Equal(expected.ModTime())) {
		return ErrSourceChanged
	}
	return nil
}
func lifecycleReadDir(root *os.Root, op sourceLifecycleIO) (entries []fs.DirEntry, err error) {
	input, err := op.open(root, ".")
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, input.Close())
		if err != nil {
			entries = nil
		}
	}()
	return input.ReadDir(-1)
}
func lifecycleReadFile(ctx context.Context, root *os.Root, name string, expected fs.FileInfo, guard func() error, op sourceLifecycleIO) (data []byte, err error) {
	if err = guard(); err != nil {
		return nil, err
	}
	if err = sourceEntryMatches(root, name, expected); err != nil {
		return nil, err
	}
	input, err := op.open(root, name)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, input.Close())
		if err != nil {
			data = nil
		}
	}()
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(info, expected) {
		return nil, ErrSourceChanged
	}
	buffer := make([]byte, 64*1024)
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = guard(); err != nil {
			return nil, err
		}
		n, readErr := input.Read(buffer)
		if n > 0 {
			data = append(data, buffer[:n]...)
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return nil, readErr
			}
			break
		}
	}
	if err = guard(); err != nil {
		return nil, err
	}
	if err = sourceEntryMatches(root, name, expected); err != nil {
		return nil, err
	}
	return data, nil
}
