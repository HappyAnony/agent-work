// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package historyindex

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"wiki-kernel/internal/indexrepair"
)

type historyDirectory interface {
	ReadDir(int) ([]fs.DirEntry, error)
	Stat() (fs.FileInfo, error)
	Close() error
}

// Trusted, invocation-local ports. Reading does not acquire a writer capability.
type historyScanIO struct {
	open func(*os.Root, string) (historyDirectory, error)
	read func(context.Context, indexrepair.Source, string) ([]byte, error)
}

func (op historyScanIO) defaults() historyScanIO {
	if op.open == nil {
		op.open = func(root *os.Root, name string) (historyDirectory, error) { return root.Open(name) }
	}
	if op.read == nil {
		op.read = func(ctx context.Context, source indexrepair.Source, name string) ([]byte, error) {
			return source.Read(ctx, name)
		}
	}
	return op
}

type historyEntry struct {
	name     string
	info     fs.FileInfo
	children []string
}
type historyManifest struct {
	source indexrepair.Source
	root   fs.FileInfo
	nodes  []*historyEntry // lexical depth-first order, matching fs.WalkDir
	byName map[string]*historyEntry
	op     historyScanIO
}

func (m *historyManifest) checkEntry(ctx context.Context, saved *historyEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := m.source.Data.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(root, m.root) {
		return indexrepair.ErrSourceChanged
	}
	if err = m.source.Check(saved.name, false); err != nil {
		return err
	}
	now, err := m.source.Data.Lstat(saved.name)
	if err != nil {
		return err
	}
	if now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(now, saved.info) || now.Mode().Type() != saved.info.Mode().Type() || now.Size() != saved.info.Size() || !now.ModTime().Equal(saved.info.ModTime()) {
		return indexrepair.ErrSourceChanged
	}
	return ctx.Err()
}

func (m *historyManifest) directory(ctx context.Context, saved *historyEntry) (names []string, err error) {
	if err = m.checkEntry(ctx, saved); err != nil {
		return nil, err
	}
	input, err := m.op.open(m.source.Data, saved.name)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, input.Close(), m.checkEntry(ctx, saved))
		if err != nil {
			names = nil
		}
	}()
	verify := func() error {
		if err := m.checkEntry(ctx, saved); err != nil {
			return err
		}
		info, err := input.Stat()
		if err != nil {
			return err
		}
		if !info.IsDir() || !os.SameFile(info, saved.info) || info.Size() != saved.info.Size() || !info.ModTime().Equal(saved.info.ModTime()) {
			return indexrepair.ErrSourceChanged
		}
		return nil
	}
	seen := map[string]bool{}
	for {
		if err = verify(); err != nil {
			return nil, err
		}
		entries, readErr := input.ReadDir(64)
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
			if !fs.ValidPath(n) || n == "." || path.Base(n) != n || strings.Contains(n, "\\") || seen[n] {
				return nil, indexrepair.ErrSourcePath
			}
			seen[n] = true
			names = append(names, n)
		}
		if readErr == io.EOF {
			break
		}
		if len(entries) == 0 {
			return nil, io.ErrNoProgress
		}
	}
	slices.Sort(names)
	return names, nil
}

func captureHistoryManifest(ctx context.Context, source indexrepair.Source, name string, op historyScanIO) (*historyManifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := source.Check(name, false); err != nil {
		return nil, err
	}
	root, err := source.Data.Stat(".")
	if err != nil {
		return nil, err
	}
	m := &historyManifest{source: source, root: root, byName: map[string]*historyEntry{}, op: op}
	var walk func(string) error
	walk = func(p string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := source.Check(p, false); err != nil {
			return err
		}
		info, err := source.Data.Lstat(p)
		if err != nil {
			return err
		}
		if p == name && !info.IsDir() {
			return ErrDirectory
		}
		saved := &historyEntry{name: p, info: info}
		m.nodes = append(m.nodes, saved)
		m.byName[p] = saved
		if info.IsDir() {
			saved.children, err = m.directory(ctx, saved)
			if err != nil {
				return err
			}
			for _, child := range saved.children {
				if err = walk(path.Join(p, child)); err != nil {
					return err
				}
			}
		}
		return m.checkEntry(ctx, saved)
	}
	if err = walk(name); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *historyManifest) check(ctx context.Context) error {
	for _, saved := range m.nodes {
		if err := m.checkEntry(ctx, saved); err != nil {
			return err
		}
		if saved.info.IsDir() {
			names, err := m.directory(ctx, saved)
			if err != nil {
				return err
			}
			if !slices.Equal(names, saved.children) {
				return indexrepair.ErrSourceChanged
			}
		}
	}
	// A later directory Close may mutate an earlier entry: recheck all metadata
	// after the last borrowed descriptor has closed, before delivering candidates.
	for _, saved := range m.nodes {
		if err := m.checkEntry(ctx, saved); err != nil {
			return err
		}
	}
	return nil
}
