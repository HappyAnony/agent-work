// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"wiki-kernel/internal/pdfdevice"
	"wiki-kernel/util"
)

// Committed distinguishes failure before rename from failure after replacement.
// Code is safe for the HTTP envelope; the full cause remains available via Unwrap.
type PDFProcessError struct {
	Code      string
	Committed bool
	Err       error
}

func (e *PDFProcessError) Error() string {
	if e.Committed {
		return e.Code + " (PDF file already updated)"
	}
	return e.Code
}
func (e *PDFProcessError) Unwrap() error { return e.Err }
func pdfProcessFailure(code string, committed bool, e error) error {
	if e == nil {
		return nil
	}
	return &PDFProcessError{Code: code, Committed: committed, Err: e}
}

type pdfFileOperations struct {
	write               func(*os.File, []byte) error
	syncFile, closeFile func(*os.File) error
	rename              func(*os.Root, string, string) error
	syncDir             func(*os.Root) error
}

func defaultPDFFileOperations() pdfFileOperations {
	return pdfFileOperations{
		write: func(f *os.File, p []byte) error { _, e := io.Copy(f, bytes.NewReader(p)); return e }, syncFile: func(f *os.File) error { return f.Sync() }, closeFile: func(f *os.File) error { return f.Close() }, rename: func(r *os.Root, a, b string) error { return r.Rename(a, b) }, syncDir: func(r *os.Root) error {
			f, e := r.Open(".")
			if e != nil {
				return e
			}
			return errors.Join(f.Sync(), f.Close())
		},
	}
}

type pdfFileSelection struct {
	m                              *markdownExportContext
	selectedParent, physical, name string
	root                           *os.Root
	parentInfo, info               os.FileInfo
	digest                         [32]byte
	stage                          string
	stageInfo                      os.FileInfo
	committed                      bool
}

func (m *markdownExportContext) capturePDFFile(selected string) (s *pdfFileSelection, err error) {
	if e := m.guard(); e != nil {
		return nil, e
	}
	if !filepath.IsAbs(selected) || strings.Contains(selected, "\\") || !strings.EqualFold(filepath.Ext(selected), ".pdf") {
		return nil, errors.New("pdf_file_path_invalid")
	}
	for _, part := range strings.Split(selected, string(os.PathSeparator)) {
		if part == ".." {
			return nil, errors.New("pdf_file_path_invalid")
		}
	}
	selected = filepath.Clean(selected)
	parent := filepath.Dir(selected)
	physical, e := filepath.EvalSymlinks(parent)
	if e != nil {
		return nil, e
	}
	target := filepath.Join(physical, filepath.Base(selected))
	r := m.read.binding.scope.commit.r
	protected := []string{r.workspace, r.data, m.read.temp, util.RepoDir, util.HistoryDir, m.htmlResources.work.path, m.htmlResources.theme.path, m.htmlResources.icon.path, m.htmlResources.fallback.path}
	for _, p := range protected {
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			return nil, errors.New("pdf_file_protected_root_invalid")
		}
		_, base, tail, e := htmlDesktopAncestor(filepath.Clean(p))
		if e != nil {
			return nil, e
		}
		if htmlPathContains(filepath.Join(base, tail), target) {
			return nil, errors.New("pdf_file_destination_overlaps_source")
		}
	}
	root, e := openArchivePublicationSource(physical)
	if e != nil {
		return nil, e
	}
	keep := false
	defer func() {
		if !keep {
			err = errors.Join(err, root.Close())
		}
	}()
	parentInfo, e := root.Stat(".")
	if e != nil {
		return nil, e
	}
	info, e := root.Lstat(filepath.Base(selected))
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("pdf_file_not_regular")
	}
	if info.Size() > pdfdevice.MaxPDF {
		return nil, errors.New("pdf_file_input_limit")
	}
	if f := m.pdfWatermark.media.info; f != nil && os.SameFile(info, f) {
		return nil, errors.New("pdf_file_overlaps_watermark_source")
	}
	s = &pdfFileSelection{m: m, selectedParent: parent, physical: physical, name: filepath.Base(selected), root: root, parentInfo: parentInfo, info: info}
	if e = s.checkOriginal(); e != nil {
		return nil, e
	}
	keep = true
	return s, nil
}
func (s *pdfFileSelection) checkParent() error {
	if e := s.m.guard(); e != nil {
		return e
	}
	physical, e := filepath.EvalSymlinks(s.selectedParent)
	if e != nil {
		return e
	}
	if physical != s.physical {
		return errors.New("pdf_file_parent_changed")
	}
	info, e := os.Lstat(s.physical)
	if e = sameArchiveDirectory(info, e, s.parentInfo); e != nil {
		return e
	}
	info, e = s.root.Stat(".")
	return sameArchiveDirectory(info, e, s.parentInfo)
}
func samePDFFile(info, want os.FileInfo) bool {
	return info != nil && want != nil && info.Mode().IsRegular() && os.SameFile(info, want) && info.Size() == want.Size() && info.ModTime().Equal(want.ModTime()) && info.Mode() == want.Mode()
}
func (s *pdfFileSelection) checkOriginal() error {
	if e := s.checkParent(); e != nil {
		return e
	}
	info, e := s.root.Lstat(s.name)
	if e != nil {
		return e
	}
	if !samePDFFile(info, s.info) {
		return errors.New("pdf_file_input_changed")
	}
	return nil
}
func (s *pdfFileSelection) read() (raw []byte, err error) {
	if err = s.checkOriginal(); err != nil {
		return nil, err
	}
	// Open without truncation, but require the OS to grant the original write
	// access too. Atomic rename must not silently bypass a read-only input.
	f, err := s.root.OpenFile(s.name, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, f.Close(), s.checkOriginal())
		if err != nil {
			raw = nil
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !samePDFFile(info, s.info) {
		return nil, errors.New("pdf_file_input_changed")
	}
	budget := int64(pdfdevice.MaxPDF + 1)
	reader := &pdfAssetReader{reader: f, guard: s.checkOriginal, remaining: budget, total: &budget}
	raw, err = io.ReadAll(io.LimitReader(reader, pdfdevice.MaxPDF+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > pdfdevice.MaxPDF {
		return nil, errors.New("pdf_file_input_limit")
	}
	return raw, nil
}
func (s *pdfFileSelection) clean() (err error) {
	if s.stage != "" {
		info, e := s.root.Lstat(s.stage)
		if !os.IsNotExist(e) {
			if e != nil {
				err = e
			} else if s.stageInfo == nil || !info.Mode().IsRegular() || !os.SameFile(info, s.stageInfo) {
				err = errors.New("pdf_file_stage_changed")
			} else {
				err = s.root.Remove(s.stage)
			}
		}
	}
	return errors.Join(err, s.root.Close())
}
func (s *pdfFileSelection) publish(output []byte, ops pdfFileOperations) (err error) {
	if len(output) > pdfdevice.MaxOutput || !bytes.HasPrefix(output, []byte("%PDF-")) {
		return errors.New("pdf_file_output_invalid")
	}
	if ops.write == nil || ops.syncFile == nil || ops.closeFile == nil || ops.rename == nil || ops.syncDir == nil {
		return errors.New("pdf_file_operations_unbound")
	}
	if err = s.checkOriginal(); err != nil {
		return err
	}
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		return err
	}
	s.stage = ".wiki-pdf-" + hex.EncodeToString(random) + ".part"
	f, err := s.root.OpenFile(s.stage, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		s.stage = ""
		return err
	}
	s.stageInfo, err = f.Stat()
	if err != nil {
		err = errors.Join(err, ops.closeFile(f))
		return err
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, ops.closeFile(f))
		}
	}()
	for offset := 0; offset < len(output); offset += 64 * 1024 {
		if err = s.checkParent(); err != nil {
			return err
		}
		now, e := s.root.Lstat(s.stage)
		if e != nil {
			return e
		}
		if !now.Mode().IsRegular() || !os.SameFile(now, s.stageInfo) {
			return errors.New("pdf_file_stage_changed")
		}
		end := offset + 64*1024
		if end > len(output) {
			end = len(output)
		}
		if err = ops.write(f, output[offset:end]); err != nil {
			return err
		}
	}
	if err = f.Chmod(s.info.Mode().Perm()); err != nil {
		return err
	}
	if err = ops.syncFile(f); err != nil {
		return err
	}
	staged, err := f.Stat()
	if err != nil {
		return err
	}
	if staged.Size() != int64(len(output)) {
		return errors.New("pdf_file_stage_incomplete")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	hash := sha256.New()
	budget := int64(pdfdevice.MaxOutput + 1)
	reader := &pdfAssetReader{reader: f, remaining: budget, total: &budget, guard: func() error {
		if e := s.checkParent(); e != nil {
			return e
		}
		now, e := s.root.Lstat(s.stage)
		if e != nil {
			return e
		}
		if !samePDFFile(now, staged) {
			return errors.New("pdf_file_stage_changed")
		}
		return nil
	}}
	if _, err = io.Copy(hash, reader); err != nil {
		return err
	}
	if !bytes.Equal(hash.Sum(nil), sumPDF(output)) {
		return errors.New("pdf_file_stage_changed")
	}
	err = ops.closeFile(f)
	closed = true
	if err != nil {
		return err
	}
	// Re-read the original just before commit: inode/size/mtime checks alone
	// cannot detect a same-size edit whose timestamp was restored.
	current, err := s.read()
	if err != nil {
		return err
	}
	if sha256.Sum256(current) != s.digest {
		return errors.New("pdf_file_input_changed")
	}
	if err = s.checkOriginal(); err != nil {
		return err
	}
	now, err := s.root.Lstat(s.stage)
	if err != nil {
		return err
	}
	if !samePDFFile(now, staged) {
		return errors.New("pdf_file_stage_changed")
	}
	if err = ops.rename(s.root, s.stage, s.name); err != nil {
		return err
	}
	s.committed = true
	if err = ops.syncDir(s.root); err != nil {
		return err
	}
	if err = s.checkParent(); err != nil {
		return err
	}
	now, err = s.root.Lstat(s.name)
	if err != nil {
		return err
	}
	if !samePDFFile(now, staged) {
		return errors.New("pdf_file_replacement_changed")
	}
	return nil
}
func sumPDF(raw []byte) []byte { sum := sha256.Sum256(raw); return sum[:] }
func (m *markdownExportContext) processPDFFile(id, selected string, merge, embed, watermark bool, device pdfDeviceBinding, ops pdfFileOperations) (err error) {
	d, e := m.preparePDFPostprocessDocument(id, merge)
	if e != nil {
		return pdfProcessFailure("pdf_document_failed", false, e)
	}
	file, e := m.capturePDFFile(selected)
	if e != nil {
		return pdfProcessFailure("pdf_file_selection_failed", false, e)
	}
	defer func() {
		if e := file.clean(); e != nil {
			err = errors.Join(err, pdfProcessFailure("pdf_file_cleanup_failed", file.committed, e))
		}
	}()
	raw, e := file.read()
	if e != nil {
		return pdfProcessFailure("pdf_file_read_failed", false, e)
	}
	file.digest = sha256.Sum256(raw)
	output, e := d.processPDFBytes(raw, embed, watermark, device)
	if e != nil {
		return pdfProcessFailure("pdf_processing_failed", false, e)
	}
	if e = file.publish(output, ops); e != nil {
		return pdfProcessFailure("pdf_file_publish_failed", file.committed, e)
	}
	return nil
}
func ProcessPDFContext(ctx context.Context, id, selected string, merge, embed, watermark bool) error {
	m, e := captureMarkdownExport(ctx)
	if e != nil {
		return pdfProcessFailure("pdf_context_failed", false, e)
	}
	executable, e := os.Executable()
	if e != nil {
		return pdfProcessFailure("pdf_device_binding_failed", false, e)
	}
	execute, e := pdfdevice.Bind(executable)
	if e != nil {
		return pdfProcessFailure("pdf_device_binding_failed", false, e)
	}
	return m.processPDFFile(id, selected, merge, embed, watermark, pdfDeviceBinding{execute: execute, version: util.Ver}, defaultPDFFileOperations())
}

// Joined cleanup errors can carry a later committed state than the first cause.
func PDFProcessCommitted(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*PDFProcessError); ok && e.Committed {
		return true
	}
	if e, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range e.Unwrap() {
			if PDFProcessCommitted(cause) {
				return true
			}
		}
	}
	if e, ok := err.(interface{ Unwrap() error }); ok {
		return PDFProcessCommitted(e.Unwrap())
	}
	return false
}
