// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path"
	"wiki-kernel/internal/indexrepair"
)

// Keep original resource overwrite semantics, with bounded copying and atomic
// scratch-file replacement. This is not cross-process CAS or a source snapshot.
func (e *archiveExportContext) copyExportFile(source, target indexrepair.Source, from, to string) (err error) {
	if err = e.guard(); err != nil {
		return err
	}
	ctx := e.binding.scope.ctx
	if target.Authorize == nil {
		return indexrepair.ErrSourceWrite
	}
	if err = target.Authorize(ctx); err != nil {
		return err
	}
	if err = source.Check(from, false); err != nil {
		return err
	}
	info, err := source.Data.Stat(from)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return indexrepair.ErrSourcePath
	}
	if err = target.Check(to, true); err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if old, statErr := target.Data.Lstat(to); statErr == nil {
		if !old.Mode().IsRegular() {
			return indexrepair.ErrSourcePath
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	if err = target.Data.MkdirAll(path.Dir(to), 0755); err != nil {
		return err
	}
	input, err := source.Data.Open(from)
	if err != nil {
		return err
	}
	tmp := path.Join(path.Dir(to), ".wiki-export-"+ast.NewNodeID())
	output, err := target.Data.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return errors.Join(err, input.Close())
	}
	defer func() {
		removeErr := target.Data.Remove(tmp)
		if !os.IsNotExist(removeErr) {
			err = errors.Join(err, removeErr)
		}
	}()
	err = output.Chmod(info.Mode())
	if err == nil {
		err = archiveCopy(ctx, output, input)
	} // Shared 64 KiB, cancellation-aware copy loop.
	err = errors.Join(err, input.Close())
	if err == nil {
		err = target.Data.Chtimes(tmp, info.ModTime(), info.ModTime())
	}
	if err == nil {
		err = output.Sync()
	}
	err = errors.Join(err, output.Close())
	if err != nil {
		return err
	}
	if err = e.guard(); err != nil {
		return err
	}
	if err = target.Authorize(ctx); err != nil {
		return err
	}
	if err = target.Check(to, true); err != nil {
		return err
	}
	return target.Data.Rename(tmp, to)
}
