// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/88250/lute/ast"
)

type notebookWalkIO struct {
	readDir func(context.Context, Source, string) ([]fs.DirEntry, error)
}

type notebookManifestEntry struct {
	name string
	info fs.FileInfo
}

// Documents lists canonical source documents only, preserving deterministic
// filesystem order and the original invalid-name/hidden/asset skipping rules.
func (s Source) Documents(ctx context.Context, box string) (out []DocumentPath, err error) {
	return s.documentsWithIO(ctx, box, notebookWalkIO{})
}

func (s Source) documentManifest(ctx context.Context, box string, op notebookWalkIO) ([]notebookManifestEntry, error) {
	if !ast.IsNodeIDPattern(box) {
		return nil, ErrSourcePath
	}
	if err := s.Check(box, false); err != nil {
		return nil, err
	}
	rootInfo, err := s.Data.Lstat(box)
	if err != nil {
		return nil, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&fs.ModeSymlink != 0 {
		return nil, ErrSourcePath
	}
	readDir := op.readDir
	if readDir == nil {
		readDir = func(ctx context.Context, source Source, name string) ([]fs.DirEntry, error) {
			return source.ReadDir(ctx, name)
		}
	}
	manifest := []notebookManifestEntry{{name: box, info: rootInfo}}
	var scan func(string) error
	scan = func(folder string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		before, err := s.Data.Lstat(folder)
		if err != nil {
			return err
		}
		if !before.IsDir() || before.Mode()&fs.ModeSymlink != 0 {
			return ErrSourcePath
		}
		entries, err := readDir(ctx, s, folder)
		if err != nil {
			return err
		}
		after, err := s.Data.Lstat(folder)
		if err != nil {
			return err
		}
		if !after.IsDir() || after.Mode()&fs.ModeSymlink != 0 || !os.SameFile(after, before) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return ErrSourceChanged
		}
		for _, entry := range entries {
			if err = ctx.Err(); err != nil {
				return err
			}
			name := path.Join(folder, entry.Name())
			if err = s.Check(name, false); err != nil {
				return err
			}
			info, statErr := s.Data.Lstat(name)
			if statErr != nil {
				return statErr
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return ErrSourcePath
			}
			if info.IsDir() != entry.IsDir() || info.Mode().Type() != entry.Type() {
				return ErrSourceChanged
			}
			if info.IsDir() {
				if !ast.IsNodeIDPattern(entry.Name()) {
					continue
				}
				manifest = append(manifest, notebookManifestEntry{name: name, info: info})
				if err = scan(name); err != nil {
					return err
				}
				continue
			}
			if !strings.HasSuffix(name, ".docj") || !ast.IsNodeIDPattern(strings.TrimSuffix(path.Base(name), ".docj")) {
				continue
			}
			if !info.Mode().IsRegular() {
				return ErrSourcePath
			}
			manifest = append(manifest, notebookManifestEntry{name: name, info: info})
		}
		return ctx.Err()
	}
	if err = scan(box); err != nil {
		return nil, err
	}
	for _, entry := range manifest {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = s.Check(entry.name, false); err != nil {
			return nil, err
		}
		info, statErr := s.Data.Lstat(entry.name)
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode()&fs.ModeSymlink != 0 || info.IsDir() != entry.info.IsDir() || info.Mode().Type() != entry.info.Mode().Type() || !os.SameFile(info, entry.info) || info.Size() != entry.info.Size() || !info.ModTime().Equal(entry.info.ModTime()) {
			return nil, ErrSourceChanged
		}
	}
	return manifest, ctx.Err()
}

func (s Source) documentsWithIO(ctx context.Context, box string, op notebookWalkIO) ([]DocumentPath, error) {
	manifest, err := s.documentManifest(ctx, box, op)
	if err != nil {
		return nil, err
	}
	var out []DocumentPath
	for _, entry := range manifest {
		if entry.info.IsDir() {
			continue
		}
		docs, parseErr := DocumentPaths([]string{entry.name})
		if parseErr != nil {
			return nil, parseErr
		}
		out = append(out, docs...)
	}
	return out, nil
}
