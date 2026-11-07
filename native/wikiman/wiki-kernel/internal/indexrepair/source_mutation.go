// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"sort"
)

type sourceMutationIO struct {
	lifecycle sourceLifecycleIO
	rename    func(*os.Root, string, string) error
	expected  []byte
	compare   bool
}

func (op sourceMutationIO) defaults() sourceMutationIO {
	op.lifecycle = op.lifecycle.defaults()
	if op.rename == nil {
		op.rename = func(root *os.Root, from, to string) error { return root.Rename(from, to) }
	}
	return op
}
func (s Source) mutationEntry(name string) (fs.FileInfo, error) {
	if err := s.Check(name, false); err != nil {
		return nil, err
	}
	info, err := s.Data.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return nil, ErrSourcePath
	}
	return info, nil
}
func (s Source) mutationParent(name string) (fs.FileInfo, error) {
	folder := path.Dir(name)
	if folder != "." {
		if err := s.Check(folder, false); err != nil {
			return nil, err
		}
	}
	info, err := s.Data.Lstat(folder)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, ErrSourcePath
	}
	return info, nil
}
func sourceTargetAbsent(s Source, name string) error {
	if err := s.Check(name, true); err != nil {
		return err
	}
	_, err := s.Data.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err == nil {
		return ErrSourceChanged
	}
	return err
}

// Locks are optional trusted bindings. Stable path order coordinates all
// cooperating Source mutators; it is not a cross-process filesystem fence.
func (s Source) lockMutationPaths(names ...string) func() {
	sort.Strings(names)
	var unlocks []func()
	for i, name := range names {
		if i == 0 || name != names[i-1] {
			unlocks = append(unlocks, s.locked(name))
		}
	}
	return func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
}
func (s Source) remove(ctx context.Context, name string, op sourceMutationIO) (err error) {
	if err = s.write(ctx); err != nil {
		return err
	}
	identity, err := s.mutationEntry(name)
	if err != nil {
		return err
	}
	initialParent, err := s.mutationParent(name)
	if err != nil {
		return err
	}
	unlock := s.lockMutationPaths(name)
	defer unlock()
	if err = s.write(ctx); err != nil {
		return err
	}
	op = op.defaults()
	parent, err := borrowSourceDirectory(s, path.Dir(name), op.lifecycle)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, op.lifecycle.closeRoot(parent.root)) }()
	if !os.SameFile(parent.identity, initialParent) {
		return ErrSourceChanged
	}
	if err = s.write(ctx); err != nil {
		return err
	}
	if err = parent.check(); err != nil {
		return err
	}
	base := path.Base(name)
	if err = sourceEntryMatches(parent.root, base, identity); err != nil {
		return err
	}
	if op.compare {
		if !identity.Mode().IsRegular() {
			return ErrSourcePath
		}
		actual, e := lifecycleReadFile(ctx, parent.root, base, identity, parent.check, op.lifecycle)
		if e != nil {
			return e
		}
		if !bytes.Equal(actual, op.expected) {
			return ErrSourceChanged
		}
		if err = sourceEntryMatches(parent.root, base, identity); err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = op.lifecycle.remove(parent.root, base); err != nil {
		return err
	}
	return parent.check()
}
func (s Source) rename(ctx context.Context, from, to string, op sourceMutationIO) (err error) {
	if err = s.write(ctx); err != nil {
		return err
	}
	identity, err := s.mutationEntry(from)
	if err != nil {
		return err
	}
	if err = sourceTargetAbsent(s, to); err != nil {
		return err
	}
	initialSource, err := s.mutationParent(from)
	if err != nil {
		return err
	}
	initialTarget, err := s.mutationParent(to)
	if err != nil {
		return err
	}
	unlock := s.lockMutationPaths(from, to)
	defer unlock()
	if err = s.write(ctx); err != nil {
		return err
	}
	op = op.defaults()
	fromDir, toDir := path.Dir(from), path.Dir(to)
	source, err := borrowSourceDirectory(s, fromDir, op.lifecycle)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, op.lifecycle.closeRoot(source.root)) }()
	target := source
	if fromDir != toDir {
		target, err = borrowSourceDirectory(s, toDir, op.lifecycle)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, op.lifecycle.closeRoot(target.root)) }()
	}
	if !os.SameFile(source.identity, initialSource) || !os.SameFile(target.identity, initialTarget) {
		return ErrSourceChanged
	}
	if err = s.write(ctx); err != nil {
		return err
	}
	if err = source.check(); err != nil {
		return err
	}
	if err = target.check(); err != nil {
		return err
	}
	if err = sourceEntryMatches(source.root, path.Base(from), identity); err != nil {
		return err
	}
	if err = sourceTargetAbsent(Source{Data: target.root}, path.Base(to)); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// A same-parent rename uses that retained directory directly. Cross-parent
	// moves remain relative to the captured data root, with both parents checked.
	// Checks + Rename still have syscall gaps against arbitrary external writers.
	if fromDir == toDir {
		err = op.rename(source.root, path.Base(from), path.Base(to))
	} else {
		err = op.rename(s.Data, from, to)
	}
	if err != nil {
		return err
	}
	if err = source.check(); err != nil {
		return err
	}
	if err = target.check(); err != nil {
		return err
	}
	return sourceEntryMatches(target.root, path.Base(to), identity)
}
