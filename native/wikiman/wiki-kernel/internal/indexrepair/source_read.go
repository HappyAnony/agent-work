// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
)

// ErrSourceReadIncomplete means a selected read or its resource cleanup failed.
// Underlying causes remain inspectable, including missing/close/cancellation.
var ErrSourceReadIncomplete = errors.New("repair_source_read_incomplete")
var ErrSourceReadLimit = errors.New("repair_source_read_limit")

// ReadLimited retains the checked read contract while bounding payload bytes.
// A negative bound is invalid; zero admits only an existing empty file.
func (s Source) ReadLimited(ctx context.Context, name string, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return nil, ErrSourceReadLimit
	}
	return s.read(ctx, name, sourceReadIO{limited: true, maxBytes: maxBytes})
}

// IsSourceNotExist admits only pure initial absence. Compound failures and an
// incomplete selected read must not authorize default data or new-file writes.
func IsSourceNotExist(err error) bool {
	if err == nil || errors.Is(err, ErrSourceReadIncomplete) {
		return false
	}
	if group, ok := err.(interface{ Unwrap() []error }); ok {
		causes := group.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !IsSourceNotExist(cause) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return IsSourceNotExist(wrapped.Unwrap())
	}
	return errors.Is(err, fs.ErrNotExist)
}

type sourceReadFile interface {
	io.Reader
	Stat() (fs.FileInfo, error)
	Close() error
}

// Private per-invocation backends support fault tests without changing Source's
// public capability, taking a writer lock, or introducing global IO selectors.
type sourceReadIO struct {
	limited   bool
	maxBytes  int64
	open      func(*os.Root, string) (sourceReadFile, error)
	closeRoot func(*os.Root) error
}

func (op sourceReadIO) defaults() sourceReadIO {
	if op.open == nil {
		op.open = func(root *os.Root, name string) (sourceReadFile, error) { return root.Open(name) }
	}
	if op.closeRoot == nil {
		op.closeRoot = func(root *os.Root) error { return root.Close() }
	}
	return op
}
func (s Source) read(ctx context.Context, name string, op sourceReadIO) (data []byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = s.Check(name, false); err != nil {
		if canceled := ctx.Err(); canceled != nil {
			return nil, errors.Join(ErrSourceReadIncomplete, err, canceled)
		}
		return nil, err // Pure initial missing keeps its original error shape.
	}
	op = op.defaults()
	var parent *sourceDirectory
	var input sourceReadFile
	var identity fs.FileInfo
	// Close in ownership order before returning bytes. Final path checks use the
	// still-borrowed outer data root, never the parent root after it closes.
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if parent != nil {
			if err := parent.check(); err != nil {
				return err
			}
		}
		if identity != nil {
			return sourceEntryMatches(s.Data, name, identity)
		}
		return nil
	}
	defer func() {
		if input != nil {
			err = errors.Join(err, input.Close())
		}
		err = errors.Join(err, check())
		if parent != nil {
			err = errors.Join(err, op.closeRoot(parent.root))
			err = errors.Join(err, check())
		}
		if err != nil {
			data = nil
			err = errors.Join(ErrSourceReadIncomplete, err)
		}
	}()
	identity, err = s.Data.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !identity.Mode().IsRegular() {
		return nil, ErrSourcePath
	}
	if op.limited && identity.Size() > op.maxBytes {
		return nil, ErrSourceReadLimit
	}
	initialParent, err := s.Data.Lstat(path.Dir(name))
	if err != nil {
		return nil, err
	}
	if !initialParent.IsDir() {
		return nil, ErrSourcePath
	}
	parent, err = borrowSourceDirectory(s, path.Dir(name), sourceLifecycleIO{closeRoot: op.closeRoot})
	if err != nil {
		return nil, err
	}
	if !os.SameFile(parent.identity, initialParent) {
		return nil, ErrSourceChanged
	}
	if err = check(); err != nil {
		return nil, err
	}
	input, err = op.open(parent.root, path.Base(name))
	if err != nil {
		return nil, err
	}
	verifyFile := func() error {
		opened, err := input.Stat()
		if err != nil {
			return err
		}
		if !opened.Mode().IsRegular() || !os.SameFile(opened, identity) || opened.Size() != identity.Size() || !opened.ModTime().Equal(identity.ModTime()) {
			return ErrSourceChanged
		}
		return check()
	}
	if err = verifyFile(); err != nil {
		return nil, err
	}
	data = make([]byte, 0) // Empty existing files remain distinct from nil-before.
	buffer := make([]byte, 64*1024)
	for {
		if err = check(); err != nil {
			return nil, err
		}
		chunk := buffer
		if op.limited && op.maxBytes-int64(len(data)) < int64(len(buffer)) {
			chunk = buffer[:op.maxBytes-int64(len(data))+1]
		}
		n, readErr := input.Read(chunk)
		if checkErr := verifyFile(); checkErr != nil {
			if readErr == io.EOF {
				readErr = nil
			}
			return nil, errors.Join(readErr, checkErr)
		}
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		if op.limited && int64(len(data))+int64(n) > op.maxBytes {
			return nil, ErrSourceReadLimit
		}
		if n > 0 {
			data = append(data, buffer[:n]...)
		}
		if readErr == io.EOF {
			break
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
	if int64(len(data)) < identity.Size() {
		return nil, io.ErrUnexpectedEOF
	}
	if int64(len(data)) > identity.Size() {
		return nil, ErrSourceChanged
	}
	return data, check()
}
