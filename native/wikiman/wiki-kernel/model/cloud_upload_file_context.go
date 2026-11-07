// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"

	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/indexrepair"
)

type cloudUploadInput interface {
	io.Reader
	Stat() (fs.FileInfo, error)
	Close() error
}
type cloudUploadIO struct {
	openRoot  func(string) (*os.Root, error)
	closeRoot func(*os.Root) error
	openFile  func(*os.Root, string) (cloudUploadInput, error)
}

func (op cloudUploadIO) defaults() cloudUploadIO {
	if op.openRoot == nil {
		op.openRoot = os.OpenRoot
	}
	if op.closeRoot == nil {
		op.closeRoot = func(r *os.Root) error { return r.Close() }
	}
	if op.openFile == nil {
		op.openFile = func(r *os.Root, n string) (cloudUploadInput, error) { return r.Open(n) }
	}
	return op
}

type cloudUploadError struct {
	message string
	cause   error
}

func (e *cloudUploadError) Error() string { return e.message }
func (e *cloudUploadError) Unwrap() error { return e.cause }

// Payload opening stays lazy: oversized assets are skipped without opening them.
// A reader belongs to one serialized upload operation, including its Close.
type cloudUploadFile struct {
	ctx      context.Context
	root     *os.Root
	name     string
	identity fs.FileInfo
	guard    func() error
	open     func(*os.Root, string) (cloudUploadInput, error)
	file     cloudUploadInput
	closed   bool
	closeErr error
}

func (f *cloudUploadFile) check() error {
	if e := f.ctx.Err(); e != nil {
		return e
	}
	if e := f.guard(); e != nil {
		return e
	}
	if e := (indexrepair.Source{Data: f.root}).Check(f.name, false); e != nil {
		return e
	}
	now, e := f.root.Lstat(f.name)
	if e != nil {
		return e
	}
	if !now.Mode().IsRegular() || !os.SameFile(now, f.identity) || now.Size() != f.identity.Size() || !now.ModTime().Equal(f.identity.ModTime()) {
		return cloudaccount.ErrAssetChanged
	}
	return nil
}
func (f *cloudUploadFile) verify() error {
	if e := f.check(); e != nil {
		return e
	}
	info, e := f.file.Stat()
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || !os.SameFile(info, f.identity) || info.Size() != f.identity.Size() || !info.ModTime().Equal(f.identity.ModTime()) {
		return cloudaccount.ErrAssetChanged
	}
	return nil
}
func (f *cloudUploadFile) Read(p []byte) (int, error) {
	if f.closed {
		return 0, os.ErrClosed
	}
	if e := f.check(); e != nil {
		return 0, e
	}
	if f.file == nil {
		input, e := f.open(f.root, f.name)
		if e != nil {
			return 0, e
		}
		f.file = input
	}
	if e := f.verify(); e != nil {
		return 0, e
	}
	n, readErr := f.file.Read(p)
	if e := f.verify(); e != nil {
		if readErr == io.EOF {
			readErr = nil
		}
		return 0, errors.Join(readErr, e)
	}
	if readErr != nil && readErr != io.EOF {
		return 0, readErr
	}
	if n == 0 && readErr == nil && len(p) > 0 {
		return 0, io.ErrNoProgress
	}
	return n, readErr
}
func (f *cloudUploadFile) Close() error {
	if f.closed {
		return f.closeErr
	}
	f.closed = true
	if f.file != nil {
		f.closeErr = f.file.Close()
	}
	f.closeErr = errors.Join(f.closeErr, f.check())
	return f.closeErr
}
