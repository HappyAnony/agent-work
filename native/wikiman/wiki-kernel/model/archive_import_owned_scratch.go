// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/ast"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"wiki-kernel/internal/indexrepair"
)

type archiveImportScratch struct {
	e                        *ArchiveImportContext
	parent, root             *os.Root
	parentInfo, info         os.FileInfo
	parentPath, name, folder string
	once                     sync.Once
	closeErr                 error
}

func (d *archiveImportScratch) check() error {
	info, err := os.Lstat(d.parentPath)
	if err = sameArchiveDirectory(info, err, d.parentInfo); err != nil {
		return err
	}
	info, err = d.parent.Lstat(d.name)
	return sameArchiveDirectory(info, err, d.info)
}

// Cleanup deliberately does not require a live request or the currently selected
// temp root. Only the captured parent's matching directory entry may be removed.
func (d *archiveImportScratch) close() error {
	if d == nil {
		return nil
	}
	d.once.Do(func() {
		d.e.scratchMu.Lock()
		if d.e.scratch[d.folder] == d {
			delete(d.e.scratch, d.folder)
		}
		d.e.scratchMu.Unlock()
		if d.root != nil {
			d.closeErr = errors.Join(d.closeErr, d.root.Close())
		}
		if d.parent != nil && d.info != nil {
			info, err := d.parent.Lstat(d.name)
			if !os.IsNotExist(err) {
				if err = sameArchiveDirectory(info, err, d.info); err != nil {
					d.closeErr = errors.Join(d.closeErr, err)
				} else {
					d.closeErr = errors.Join(d.closeErr, d.parent.RemoveAll(d.name))
				}
			}
		}
		if d.parent != nil {
			d.closeErr = errors.Join(d.closeErr, d.parent.Close())
		}
	})
	return d.closeErr
}
func (e *ArchiveImportContext) newImportScratch(prefix string) (out *archiveImportScratch, err error) {
	if err = e.guard(); err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(e.binding.scope.commit.r.workspace, e.temp)
	if err != nil || rel == "." || !fs.ValidPath(filepath.ToSlash(rel)) {
		return nil, indexrepair.ErrSourcePath
	}
	workspace, err := os.OpenRoot(e.binding.scope.commit.r.workspace)
	if err != nil {
		return nil, err
	}
	if err = (indexrepair.Source{Data: workspace}).Check(filepath.ToSlash(rel), true); err == nil {
		err = workspace.MkdirAll(rel, 0755)
	}
	var parent *os.Root
	if err == nil {
		parent, err = workspace.OpenRoot(rel)
	}
	err = errors.Join(err, workspace.Close())
	if err != nil {
		if parent != nil {
			err = errors.Join(err, parent.Close())
		}
		return nil, err
	}
	d := &archiveImportScratch{e: e, parent: parent, parentPath: e.temp, name: prefix + ast.NewNodeID()}
	d.folder = filepath.Join(e.temp, d.name)
	defer func() {
		if err != nil {
			err = errors.Join(err, d.close())
			out = nil
		}
	}()
	d.parentInfo, err = parent.Stat(".")
	if err != nil {
		return nil, err
	}
	if err = parent.Mkdir(d.name, 0700); err != nil {
		return nil, err
	}
	d.info, err = parent.Lstat(d.name)
	if err != nil {
		return nil, err
	}
	d.root, err = parent.OpenRoot(d.name)
	if err != nil {
		return nil, err
	}
	e.scratchMu.Lock()
	if e.scratch == nil {
		e.scratch = map[string]*archiveImportScratch{}
	}
	e.scratch[d.folder] = d
	e.scratchMu.Unlock()
	return d, e.guard()
}
func (e *ArchiveImportContext) scratchAt(folder string) *archiveImportScratch {
	e.scratchMu.Lock()
	defer e.scratchMu.Unlock()
	return e.scratch[folder]
}
func (e *ArchiveImportContext) releaseScratch(folder string) error {
	return e.scratchAt(folder).close()
}

// Borrow while holding the registry lock so cleanup cannot close the owner before
// its independent descriptor is opened. No absolute-path reopen for owned data.
func (e *ArchiveImportContext) borrowImportScratch(folder string) (*os.Root, string, bool, error) {
	e.scratchMu.Lock()
	defer e.scratchMu.Unlock()
	for _, d := range e.scratch {
		rel, err := filepath.Rel(d.folder, folder)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if err = d.check(); err != nil {
			return nil, "", true, err
		}
		root, err := d.root.OpenRoot(".")
		return root, filepath.ToSlash(rel), true, err
	}
	return nil, "", false, nil
}
func (e *ArchiveImportContext) openImportArchive(filename string) (file *os.File, err error) {
	if err := e.guard(); err != nil {
		return nil, err
	}
	root, rel, owned, err := e.borrowImportScratch(filename)
	if err != nil {
		return nil, err
	}
	if owned {
		closeRoot := e.archiveRootCloser()
		defer func() {
			if closeErr := closeRoot(root); closeErr != nil {
				err = errors.Join(err, closeErr)
				if file != nil {
					err = errors.Join(err, file.Close())
					file = nil
				}
			}
		}()
		if err = (indexrepair.Source{Data: root}).Check(rel, false); err != nil {
			return nil, err
		}
		return root.Open(rel)
	}
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, indexrepair.ErrSourcePath
	}
	return os.Open(filename)
}
