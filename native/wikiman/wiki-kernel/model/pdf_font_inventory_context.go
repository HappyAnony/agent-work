// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/pdfdevice"
)

type pdfFontInventory struct {
	entries map[string]pdfWatermarkFile
	err     error
}

// Capture metadata only; decode a selected old cache exclusively in the child.
func capturePDFFontInventory(directory string) (s pdfFontInventory) {
	s.entries = map[string]pdfWatermarkFile{}
	base := captureHTMLResourceRoot(directory)
	if os.IsNotExist(base.err) {
		return
	}
	if base.err != nil {
		s.err = base.err
		return
	}
	root, e := base.open()
	if e != nil {
		s.err = e
		return
	}
	defer func() { s.err = errors.Join(s.err, root.Close()) }()
	dir, e := root.Open(".")
	if e != nil {
		s.err = e
		return
	}
	defer func() { s.err = errors.Join(s.err, dir.Close()) }()
	entries, e := dir.ReadDir(8193)
	if e != nil && e != io.EOF {
		s.err = e
		return
	}
	if len(entries) > 8192 {
		s.err = errors.New("pdf_font_inventory_limit")
		return
	}
	for _, entry := range entries {
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".gob") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		f := capturePDFWatermarkFile(filepath.Join(base.path, entry.Name()))
		if !entry.Type().IsRegular() {
			f.err = errors.New("pdf_font_cache_not_regular")
		}
		f.root = base
		s.entries[name] = f
	}
	return
}
func (s pdfFontInventory) selected(name string, guard func() error) (pdfdevice.Font, bool, error) {
	if s.err != nil {
		return pdfdevice.Font{}, false, s.err
	}
	f, ok := s.entries[name]
	if !ok {
		return pdfdevice.Font{}, false, nil
	}
	raw, e := f.read(pdfWatermarkFontLimit, guard)
	if e != nil {
		return pdfdevice.Font{}, false, e
	}
	return pdfdevice.Font{Name: name, Cache: raw}, true, nil
}
