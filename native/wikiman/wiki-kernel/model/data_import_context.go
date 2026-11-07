// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"wiki-kernel/util"
)

// Complete data exports preserve IDs, notebook metadata and the original merge
// policy. They must never pass through the document archive ID remapper.
type DataImportContext struct {
	archive  *ArchiveImportContext
	complete func() error // Trusted owner admission; never a request-supplied handler.
}

func CaptureDataImport(ctx context.Context) (*DataImportContext, error) {
	e, err := CaptureArchiveImport(ctx)
	if err != nil {
		return nil, err
	}
	d := &DataImportContext{archive: e}
	d.complete = func() error {
		r := e.binding.scope.commit.r
		return r.owner.admitFullNotebookReindex(e.ctx, func() error {
			if e.temp != util.TempDir {
				return errors.New("data_import_temp_owner_changed")
			}
			return r.guard(e.ctx)
		}, func() {
			r.owner.syncRuntime.same.Store(0)
			r.owner.syncRuntime.planMu.Lock()
			r.owner.syncRuntime.plan = time.Now().Add(e.binding.scope.commit.interval)
			r.owner.syncRuntime.planMu.Unlock()
		})
	}
	return d, nil
}
func (d *DataImportContext) guard() error {
	if d == nil || d.archive == nil || d.complete == nil {
		return errors.New("data_import_context_unbound")
	}
	return d.archive.guard()
}
func (d *DataImportContext) run(work func() error) (err error) {
	if err = d.guard(); err != nil {
		return err
	}
	e := d.archive
	runtime := &e.binding.scope.commit.r.owner.syncRuntime
	runtime.lock.Lock()
	runtime.running.Store(true)
	defer func() { runtime.running.Store(false); runtime.lock.Unlock() }()
	if err = d.guard(); err != nil {
		return err
	}
	util.PushEndlessProgress(e.language(73))
	defer util.ClearPushProgress(100)
	// The inner operation closes borrowed roots and owned scratch before admission.
	if err = work(); err != nil {
		return err
	}
	if err = d.guard(); err != nil {
		return err
	}
	debug.FreeOSMemory()
	return d.complete()
}
func (d *DataImportContext) ImportFile(zipPath string) error {
	return d.run(func() error { return d.publishFile(zipPath) })
}
func (d *DataImportContext) ImportDataReader(reader io.Reader) error {
	return d.run(func() (err error) {
		if reader == nil {
			return errors.New("data_import_reader_unbound")
		}
		e := d.archive
		owned, err := e.newImportScratch("wiki-data-import-")
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, owned.close()) }()
		file, err := owned.root.OpenFile("archive.zip", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		err = archiveCopy(e.ctx, file, reader)
		err = errors.Join(err, file.Close())
		if err != nil {
			return err
		}
		return d.publishFile(filepath.Join(owned.folder, "archive.zip"))
	})
}
func (d *DataImportContext) publishFile(zipPath string) (err error) {
	e := d.archive
	scratch, folder, err := e.extractArchive(zipPath, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, e.releaseScratch(scratch)) }()
	if err = d.prepareRoot(folder); err != nil {
		return err
	}
	return e.mergeArchiveDirectory(folder, ".")
}
func (d *DataImportContext) prepareRoot(folder string) (err error) {
	e := d.archive
	source, closeIO, err := e.openScratch(folder)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIO()) }()
	// Preserve the old distinction between a document ZIP and a complete data ZIP.
	matches, err := fs.Glob(source.Data.FS(), "*.docj")
	if err != nil {
		return err
	}
	if len(matches) > 0 {
		return errors.New(e.language(198))
	}
	return e.sanitizeArchiveEmojis(source.Data, "emojis")
}
