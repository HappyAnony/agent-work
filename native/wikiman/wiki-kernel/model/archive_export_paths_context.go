// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

type archivePathIO struct {
	readDir func(context.Context, indexrepair.Source, string) ([]fs.DirEntry, error)
}
type archivePathEntry struct {
	name string
	info fs.FileInfo
}

// Retain reserved/tmp filtering and lexical recursion without deleting sources.
func (e *archiveExportContext) documentPaths(box, dir string, optional bool) ([]string, error) {
	return e.documentPathsWithIO(box, dir, optional, archivePathIO{})
}
func (e *archiveExportContext) documentPathsWithIO(box, dir string, optional bool, op archivePathIO) (out []string, err error) {
	if _, err = e.box(box); err != nil {
		return nil, err
	}
	if err = validateArchiveDestination(box, dir); err != nil {
		return nil, err
	}
	if op.readDir == nil {
		op.readDir = func(ctx context.Context, s indexrepair.Source, n string) ([]fs.DirEntry, error) {
			return s.ReadDir(ctx, n)
		}
	}
	ioctx, closeIO, err := e.binding.scope.commit.r.openRepairIOChecked(e.binding.scope.ctx, false)
	if err != nil {
		return nil, err
	}
	var base string
	var rootInfo fs.FileInfo
	var saved []archivePathEntry
	absent := ""
	check := func() error {
		if err := e.guard(); err != nil {
			return err
		}
		if rootInfo == nil {
			return nil
		}
		now, err := os.Lstat(base)
		if err != nil {
			return err
		}
		if !now.IsDir() || now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(rootInfo, now) {
			return indexrepair.ErrSourceChanged
		}
		for _, entry := range saved {
			now, err := os.Lstat(filepath.Join(base, filepath.FromSlash(entry.name)))
			if err != nil {
				return err
			}
			if now.Mode()&fs.ModeSymlink != 0 || !os.SameFile(entry.info, now) || now.Mode().Type() != entry.info.Mode().Type() || now.Size() != entry.info.Size() || !now.ModTime().Equal(entry.info.ModTime()) {
				return indexrepair.ErrSourceChanged
			}
		}
		if absent != "" {
			_, err := os.Lstat(filepath.Join(base, filepath.FromSlash(absent)))
			if indexrepair.IsSourceNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return indexrepair.ErrSourceChanged
		}
		return nil
	}
	defer func() {
		err = joinRepairCloseError(err, closeIO())
		err = joinRepairCloseError(err, check())
		if err != nil {
			out = nil
		}
	}()
	defer recoverRepositoryWorkflow(&err, "archive_export_paths")
	base, err = filepath.Abs(e.binding.scope.commit.r.data)
	if err != nil {
		return nil, err
	}
	rootInfo, err = ioctx.Source.Data.Stat(".")
	if err != nil {
		return nil, err
	}
	if err = check(); err != nil {
		return nil, err
	}
	name := path.Join(box, strings.TrimPrefix(dir, "/"))
	parts := strings.Split(name, "/")
	for i := 1; i < len(parts); i++ {
		prefix := strings.Join(parts[:i], "/")
		if err = ioctx.Source.Check(prefix, false); optional && indexrepair.IsSourceNotExist(err) {
			absent = prefix
			return nil, nil
		} else if err != nil {
			return nil, err
		}
		info, statErr := ioctx.Source.Data.Lstat(prefix)
		if statErr != nil {
			return nil, statErr
		}
		if !info.IsDir() {
			return nil, indexrepair.ErrSourcePath
		}
		saved = append(saved, archivePathEntry{prefix, info})
	}

	if err = ioctx.Source.Check(name, false); optional && indexrepair.IsSourceNotExist(err) {
		absent = name
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var scan func(string) error
	scan = func(folder string) error {
		if err := e.guard(); err != nil {
			return err
		}
		info, err := ioctx.Source.Data.Lstat(folder)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return indexrepair.ErrSourcePath
		}
		saved = append(saved, archivePathEntry{folder, info})
		entries, err := op.readDir(e.binding.scope.ctx, ioctx.Source, folder)
		if err != nil {
			return err
		}
		for _, d := range entries {
			if util.IsReservedFilename(d.Name()) || strings.HasSuffix(d.Name(), ".tmp") {
				continue
			}
			if err := e.guard(); err != nil {
				return err
			}
			next := path.Join(folder, d.Name())
			if err = ioctx.Source.Check(next, false); err != nil {
				return err
			}
			info, err := ioctx.Source.Data.Lstat(next)
			if err != nil {
				return err
			}
			if info.IsDir() {
				if err = scan(next); err != nil {
					return err
				}
			} else if util.HasDocExt(d.Name()) {
				saved = append(saved, archivePathEntry{next, info})
				out = append(out, "/"+strings.TrimPrefix(next, box+"/"))
			}
		}
		return e.guard()
	}
	if err = scan(name); err != nil {
		return nil, err
	}
	return out, check()
}
