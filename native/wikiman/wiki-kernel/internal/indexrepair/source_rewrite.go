// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
)

type sourceRewriteFile interface {
	io.Writer
	Stat() (fs.FileInfo, error)
	Sync() error
	Close() error
}

// Optional private ports support failure tests without changing Source's public
// capability or swapping process globals. Defaults are captured per operation.
type sourceRewriteIO struct {
	open      func(*os.Root, string, int, fs.FileMode) (sourceRewriteFile, error)
	rename    func(*os.Root, string, string) error
	remove    func(*os.Root, string) error
	closeRoot func(*os.Root) error
}

func (op sourceRewriteIO) defaults() sourceRewriteIO {
	if op.open == nil {
		op.open = func(root *os.Root, name string, flags int, mode fs.FileMode) (sourceRewriteFile, error) {
			return root.OpenFile(name, flags, mode)
		}
	}
	if op.rename == nil {
		op.rename = func(root *os.Root, from, to string) error { return root.Rename(from, to) }
	}
	if op.remove == nil {
		op.remove = func(root *os.Root, name string) error { return root.Remove(name) }
	}
	if op.closeRoot == nil {
		op.closeRoot = func(root *os.Root) error { return root.Close() }
	}
	return op
}
func rewriteRevision(root *os.Root, ctx context.Context, name string, before []byte) error {
	raw, err := (Source{Data: root}).Read(ctx, name)
	if before == nil {
		if IsSourceNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return ErrSourceChanged
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, before) {
		return ErrSourceChanged
	}
	return nil
}
func rewriteTempIdentity(root *os.Root, name string, identity fs.FileInfo) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if identity == nil || !info.Mode().IsRegular() || !os.SameFile(info, identity) {
		return ErrSourceChanged
	}
	return nil
}
func cleanupRewriteTemp(root *os.Root, name string, identity fs.FileInfo, remove func(*os.Root, string) error) error {
	err := rewriteTempIdentity(root, name, identity)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	err = remove(root, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// A retained parent root anchors cleanup even if its visible path is moved.
// Identity/revision checks detect observed replacements; check + rename/unlink
// do not provide cross-process CAS or exclusion against arbitrary external edits.
func (s Source) rewrite(ctx context.Context, name string, before, after []byte, op sourceRewriteIO) (err error) {
	if err = s.write(ctx); err != nil {
		return err
	}
	if err = s.Check(name, true); err != nil {
		return err
	}
	unlock := s.locked(name)
	defer unlock()
	if err = s.write(ctx); err != nil {
		return err
	}
	if err = rewriteRevision(s.Data, ctx, name, before); err != nil {
		return err
	}
	critical := context.WithoutCancel(ctx)
	if err = s.Authorize(critical); err != nil {
		return err
	}
	folder, base := path.Dir(name), path.Base(name)
	if err = s.Data.MkdirAll(folder, 0755); err != nil {
		return err
	}
	if folder != "." {
		if err = s.Check(folder, false); err != nil {
			return err
		}
	}
	visible, err := s.Data.Lstat(folder)
	if err != nil {
		return err
	}
	if !visible.IsDir() {
		return ErrSourceChanged
	}
	parent, err := s.Data.OpenRoot(folder)
	if err != nil {
		return err
	}
	op = op.defaults()
	defer func() { err = errors.Join(err, op.closeRoot(parent)) }()
	identity, err := parent.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(visible, identity) {
		return ErrSourceChanged
	}
	checkParent := func() error {
		if folder != "." {
			if err := s.Check(folder, false); err != nil {
				return err
			}
		}
		now, err := s.Data.Lstat(folder)
		if err != nil {
			return err
		}
		if !now.IsDir() || !os.SameFile(now, identity) {
			return ErrSourceChanged
		}
		return nil
	}
	if err = checkParent(); err != nil {
		return err
	}
	if err = rewriteRevision(parent, critical, base, before); err != nil {
		return err
	}
	mode := fs.FileMode(0644)
	if before != nil {
		info, statErr := parent.Stat(base)
		if statErr != nil {
			return statErr
		}
		mode = info.Mode().Perm()
	}
	id, err := nonce()
	if err != nil {
		return err
	}
	tmp := ".wiki-repair-" + id
	file, err := op.open(parent, tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	var tempIdentity fs.FileInfo
	published := false
	defer func() {
		if !published {
			err = errors.Join(err, cleanupRewriteTemp(parent, tmp, tempIdentity, op.remove))
		}
	}()
	err = func() (err error) {
		defer func() { err = errors.Join(err, file.Close()) }()
		tempIdentity, err = file.Stat()
		if err != nil {
			return err
		}
		n, err := file.Write(after)
		if err != nil {
			return err
		}
		if n != len(after) {
			return io.ErrShortWrite
		}
		return file.Sync()
	}()
	if err != nil {
		return err
	}
	if err = checkParent(); err != nil {
		return err
	}
	if err = rewriteTempIdentity(parent, tmp, tempIdentity); err != nil {
		return err
	}
	if err = rewriteRevision(parent, critical, base, before); err != nil {
		return err
	}
	if err = s.Authorize(critical); err != nil {
		return err
	}
	if err = op.rename(parent, tmp, base); err != nil {
		return err
	}
	published = true // Never remove a same-named newcomer after a successful rename.
	return nil
}
