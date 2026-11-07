// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"

	"wiki-kernel/util"
)

// One captured request owns the whole ZIP lifecycle. The filename is only a
// filtered display label; it never selects an upload or extraction filesystem path.
type MarkdownZIPImportContext struct {
	preparation *markdownImportPreparation
	importLocal func(box, folder, to, label string) error // Trusted compatibility backend.
	used        atomic.Bool
}

func CaptureMarkdownZIPImport(ctx context.Context) (*MarkdownZIPImportContext, error) {
	m, err := captureMarkdownImportPreparation(ctx)
	if err != nil {
		return nil, err
	}
	d := &MarkdownZIPImportContext{preparation: m}
	d.importLocal = func(box, folder, to, label string) error { return m.importFromLocalPath(box, folder, to, label, false) }
	return d, nil
}
func markdownZIPRootLabel(filename string) string {
	name := path.Base(strings.ReplaceAll(filename, "\\", "/"))
	name = strings.TrimSuffix(name, path.Ext(name))
	name = util.FilterFileName(name)
	if name == "" || name == "." || name == ".." {
		return "Markdown import"
	}
	return name
}
func (d *MarkdownZIPImportContext) ImportZIPReader(reader io.Reader, box, to, filename string) (err error) {
	if d == nil || d.preparation == nil || d.preparation.archive == nil || d.importLocal == nil {
		return errors.New("markdown_zip_import_unbound")
	}
	if !d.used.CompareAndSwap(false, true) {
		return errors.New("markdown_zip_import_already_used")
	}
	m := d.preparation
	defer func() { m.discard(); m.completion = nil }()
	if err = m.guard(); err != nil {
		return err
	}
	if err = validateArchiveDestination(box, to); err != nil {
		return err
	}
	if reader == nil {
		return errors.New("markdown_zip_reader_unbound")
	}
	// Inner operation joins every owned resource before any completion effects.
	err = func() (err error) {
		e := m.archive
		upload, err := e.newImportScratch("wiki-markdown-upload-")
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, upload.close()) }()
		file, err := upload.root.OpenFile("archive.zip", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		err = archiveCopy(e.ctx, file, reader)
		err = errors.Join(err, file.Close())
		if err != nil {
			return err
		}
		folder, err := e.extractArchiveContents(filepath.Join(upload.folder, "archive.zip"))
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, e.releaseScratch(folder)) }()
		if err = m.guard(); err != nil {
			return err
		}
		return d.importLocal(box, folder, to, markdownZIPRootLabel(filename))
	}()
	if err != nil {
		return err
	}
	return m.completeSync()
}
