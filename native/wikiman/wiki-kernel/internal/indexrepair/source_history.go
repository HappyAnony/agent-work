// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
)

type sourceHistoryEntry struct {
	info     fs.FileInfo
	children map[string]*sourceHistoryEntry
}

func historyChildRoot(parent *os.Root, name string, info fs.FileInfo, op sourceLifecycleIO) (root *os.Root, err error) {
	if err = sourceEntryMatches(parent, name, info); err != nil {
		return nil, err
	}
	root, err = parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err == nil && !os.SameFile(opened, info) {
		err = ErrSourceChanged
	}
	if err != nil {
		return nil, errors.Join(err, op.closeRoot(root))
	}
	return root, nil
}

// Preflight inventories the complete tree before deleting anything. During
// deletion only captured entries are removed, never a fresh recursive RemoveAll.
func scanSourceHistory(root *os.Root, guard func() error, op sourceLifecycleIO) (tree *sourceHistoryEntry, err error) {
	if err = guard(); err != nil {
		return nil, err
	}
	info, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	tree = &sourceHistoryEntry{info: info, children: map[string]*sourceHistoryEntry{}}
	entries, err := lifecycleReadDir(root, op)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err = guard(); err != nil {
			return nil, err
		}
		info, statErr := root.Lstat(entry.Name())
		if statErr != nil {
			return nil, statErr
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, ErrSourcePath
		}
		node := &sourceHistoryEntry{info: info}
		if info.IsDir() {
			child, openErr := historyChildRoot(root, entry.Name(), info, op)
			if openErr != nil {
				return nil, openErr
			}
			childGuard := func() error {
				if err := guard(); err != nil {
					return err
				}
				return sourceEntryMatches(root, entry.Name(), info)
			}
			node, err = scanSourceHistory(child, childGuard, op)
			err = errors.Join(err, op.closeRoot(child))
			if err != nil {
				return nil, err
			}
		}
		tree.children[entry.Name()] = node
	}
	return tree, guard()
}
func deleteSourceHistory(root *os.Root, tree *sourceHistoryEntry, guard func() error, op sourceLifecycleIO) (err error) {
	if err = guard(); err != nil {
		return err
	}
	entries, err := lifecycleReadDir(root, op)
	if err != nil {
		return err
	}
	if len(entries) != len(tree.children) {
		return ErrSourceChanged
	}
	for _, entry := range entries {
		saved := tree.children[entry.Name()]
		if saved == nil {
			return ErrSourceChanged
		}
		if err = sourceEntryMatches(root, entry.Name(), saved.info); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		saved := tree.children[entry.Name()]
		childGuard := func() error {
			if err := guard(); err != nil {
				return err
			}
			return sourceEntryMatches(root, entry.Name(), saved.info)
		}
		if saved.info.IsDir() {
			child, openErr := historyChildRoot(root, entry.Name(), saved.info, op)
			if openErr != nil {
				return openErr
			}
			err = deleteSourceHistory(child, saved, childGuard, op)
			err = errors.Join(err, op.closeRoot(child))
			if err != nil {
				return err
			}
		}
		if err = childGuard(); err != nil {
			return err
		}
		if err = op.remove(root, entry.Name()); err != nil {
			return err
		}
	}
	return guard()
}
func (s Source) removeLegacyHistory(ctx context.Context, box string, op sourceLifecycleIO) (err error) {
	if !fs.ValidPath(box) || strings.ContainsAny(box, "/\\") || box == "." || box == ".." {
		return ErrSourcePath
	}
	return s.removeDirectory(ctx, path.Join(box, ".siyuan", "history"), op)
}

// RemoveDirectory inventories before deletion, preserves sequential failure
// semantics and never recursively deletes entries absent from that inventory.
func (s Source) RemoveDirectory(ctx context.Context, name string) error {
	return s.removeDirectory(ctx, name, sourceLifecycleIO{})
}

func (s Source) removeDirectory(ctx context.Context, name string, op sourceLifecycleIO) (err error) {
	if !fs.ValidPath(name) || name == "." || strings.Contains(name, "\\") {
		return ErrSourcePath
	}
	if err = s.write(ctx); err != nil {
		return err
	}
	unlock := s.locked(name)
	defer unlock()
	if err = s.write(ctx); err != nil {
		return err
	}
	if err = s.Check(name, false); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	op = op.defaults()
	parent, err := borrowSourceDirectory(s, path.Dir(name), op)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, op.closeRoot(parent.root)) }()
	history, err := borrowSourceDirectory(s, name, op)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, op.closeRoot(history.root)) }()
	guard := func() error {
		if err := s.write(ctx); err != nil {
			return err
		}
		if err := parent.check(); err != nil {
			return err
		}
		return history.check()
	}
	tree, err := scanSourceHistory(history.root, guard, op)
	if err != nil {
		return err
	}
	if err = deleteSourceHistory(history.root, tree, guard, op); err != nil {
		return err
	}
	if err = guard(); err != nil {
		return err
	}
	if err = sourceEntryMatches(parent.root, path.Base(name), history.identity); err != nil {
		return err
	}
	return op.remove(parent.root, path.Base(name))
}
