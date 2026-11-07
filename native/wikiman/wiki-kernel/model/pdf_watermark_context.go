// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"io"
	"os"
	"path/filepath"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/pdfdevice"
	"wiki-kernel/util"
)

const pdfWatermarkMediaLimit int64 = 32 << 20
const pdfWatermarkFontLimit int64 = 16 << 20
const pdfWatermarkDefaultFont = "LXGWWenKaiLite-Regular"

type pdfWatermarkFile struct {
	selected, physical, name string
	root                     htmlResourceRoot
	info                     os.FileInfo
	err                      error
}
type pdfWatermarkSnapshot struct {
	value, description, mode string
	media, fallback          pdfWatermarkFile
	bound                    bool
	err                      error
}
type pdfWatermarkSpec struct {
	mode, text, description string
	media                   []byte
}

// Font metrics and PDF watermark decoding belong to the device executor. These
// callbacks are trusted host capabilities, never fields in an HTTP/CLI request.
// Their pdfcpu global-state isolation remains the executor's responsibility.
type pdfWatermarkDevice struct {
	fonts   func() ([]string, error)
	install func(string, []byte) error
	parse   func(pdfWatermarkSpec) (*pdfmodel.Watermark, error)
	apply   func(*pdfmodel.Context, *pdfmodel.Watermark) error
}

func capturePDFWatermarkFile(selected string) (f pdfWatermarkFile) {
	f.selected, f.err = filepath.Abs(selected)
	if f.err != nil {
		return
	}
	f.physical, f.err = filepath.EvalSymlinks(f.selected)
	if f.err != nil {
		return
	}
	f.info, f.err = os.Lstat(f.physical)
	if f.err != nil {
		return
	}
	if !f.info.Mode().IsRegular() {
		f.err = errors.New("pdf_watermark_source_not_regular")
		return
	}
	f.name = filepath.Base(f.physical)
	f.root = captureHTMLResourceRoot(filepath.Dir(f.physical))
	return
}
func capturePDFWatermarkSnapshot(settings conf.Export, appearance string) (s pdfWatermarkSnapshot) {
	s.bound = true
	s.value = settings.PDFWatermarkStr
	s.description = settings.PDFWatermarkDesc
	s.mode = "text"
	s.fallback = capturePDFWatermarkFile(filepath.Join(appearance, "fonts", "LxgwWenKai-Lite-1.501", pdfWatermarkDefaultFont+".ttf"))
	if s.value == "" {
		return
	}
	_, err := os.Lstat(s.value)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.err = err
		return
	}
	s.media = capturePDFWatermarkFile(s.value)
	s.mode = "image"
	if strings.EqualFold(filepath.Ext(s.value), ".pdf") {
		s.mode = "pdf"
	}
	return
}
func (f pdfWatermarkFile) read(limit int64, guard func() error) (raw []byte, err error) {
	if guard == nil {
		return nil, errors.New("pdf_watermark_guard_unbound")
	}
	if err = guard(); err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	if f.info == nil || f.info.Size() > limit {
		return nil, errors.New("pdf_watermark_file_limit")
	}
	root, err := f.root.open()
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			raw = nil
		}
	}()
	check := func() error {
		if e := guard(); e != nil {
			return e
		}
		physical, e := filepath.EvalSymlinks(f.selected)
		if e != nil {
			return e
		}
		if physical != f.physical {
			return errors.New("pdf_watermark_source_changed")
		}
		info, e := os.Lstat(f.root.path)
		if e = sameArchiveDirectory(info, e, f.root.info); e != nil {
			return e
		}
		if e = (indexrepair.Source{Data: root}).Check(f.name, false); e != nil {
			return e
		}
		info, e = root.Lstat(f.name)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || !os.SameFile(info, f.info) || info.Size() != f.info.Size() || !info.ModTime().Equal(f.info.ModTime()) {
			return errors.New("pdf_watermark_source_changed")
		}
		return nil
	}
	if err = check(); err != nil {
		return nil, err
	}
	input, err := root.Open(f.name)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, input.Close(), check())
		if err != nil {
			raw = nil
		}
	}()
	info, err := input.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(info, f.info) {
		return nil, errors.New("pdf_watermark_source_changed")
	}
	budget := limit + 1
	reader := &pdfAssetReader{reader: input, guard: check, remaining: limit + 1, total: &budget}
	raw, err = io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("pdf_watermark_file_limit")
	}
	return raw, nil
}
func pdfWatermarkFontDescription(description string, names []string) (string, bool) {
	return pdfdevice.FontDescription(description, names)
}
func applyCapturedPDFWatermark(pdf *pdfmodel.Context, s pdfWatermarkSnapshot, enabled bool, device pdfWatermarkDevice, guard func() error) error {
	if guard == nil {
		return errors.New("pdf_watermark_guard_unbound")
	}
	if e := guard(); e != nil {
		return e
	}
	if !enabled {
		return nil
	}
	if !s.bound {
		return errors.New("pdf_watermark_dependencies_unbound")
	}
	if s.value == "" {
		return nil
	}
	if pdf == nil || pdf.XRefTable == nil || device.parse == nil || device.apply == nil {
		return errors.New("pdf_watermark_dependencies_unbound")
	}
	if s.err != nil {
		return s.err
	}
	spec := pdfWatermarkSpec{mode: s.mode, text: s.value, description: s.description}
	if s.mode == "text" && util.ContainsCJK(s.value) {
		if device.fonts == nil {
			return errors.New("pdf_watermark_fonts_unbound")
		}
		names, e := device.fonts()
		if e != nil {
			return e
		}
		if e = guard(); e != nil {
			return e
		}
		description, fallback := pdfWatermarkFontDescription(s.description, names)
		spec.description = description
		if fallback {
			if device.install == nil {
				return errors.New("pdf_watermark_fonts_unbound")
			}
			raw, e := s.fallback.read(pdfWatermarkFontLimit, guard)
			if e != nil {
				return e
			}
			if e = device.install(pdfWatermarkDefaultFont, raw); e != nil {
				return e
			}
			if e = guard(); e != nil {
				return e
			}
		}
	}
	if s.mode != "text" {
		if s.mode == "image" && !pdfmodel.ImageFileName(s.value) {
			return errors.New("pdf_watermark_image_type_invalid")
		}
		raw, e := s.media.read(pdfWatermarkMediaLimit, guard)
		if e != nil {
			return e
		}
		spec.media = raw
		spec.text = ""
	}
	if e := guard(); e != nil {
		return e
	}
	wm, e := device.parse(spec)
	if e != nil {
		return e
	}
	if wm == nil {
		return errors.New("pdf_watermark_parse_empty")
	}
	wm.OnTop = true
	if e = guard(); e != nil {
		return e
	}
	if e = device.apply(pdf, wm); e != nil {
		return e
	}
	return guard()
}
func (d *pdfPostprocessDocument) watermark(pdf *pdfmodel.Context, enabled bool, device pdfWatermarkDevice) error {
	if e := d.guard(); e != nil {
		return e
	}
	return applyCapturedPDFWatermark(pdf, d.m.pdfWatermark, enabled, device, d.guard)
}

// Explicit adapter for the compatibility runtime and isolated-device tests.
// The owned model never selects this adapter as an implicit fallback.
func compatibilityPDFWatermarkDevice() pdfWatermarkDevice {
	return pdfWatermarkDevice{
		fonts: api.ListFonts,
		install: func(name string, raw []byte) error {
			if e := font.InstallFontFromBytes(font.UserFontDir, name, raw); e != nil {
				return e
			}
			return api.LoadUserFonts()
		},
		parse: func(s pdfWatermarkSpec) (*pdfmodel.Watermark, error) {
			return pdfdevice.Parse(s.mode, s.text, s.description, s.media)
		},
		apply: func(pdf *pdfmodel.Context, wm *pdfmodel.Watermark) error { return pdfcpu.AddWatermarks(pdf, nil, wm) },
	}
}
func captureLegacyPDFWatermark() (s pdfWatermarkSnapshot, err error) {
	if Conf == nil {
		return s, errors.New("pdf_watermark_preferences_unbound")
	}
	err = Conf.WithSettingSnapshot(context.Background(), func(data conf.AppData) error {
		if data.Export == nil {
			return errors.New("pdf_watermark_preferences_unbound")
		}
		s = capturePDFWatermarkSnapshot(*data.Export, util.AppearancePath)
		return nil
	})
	return
}
