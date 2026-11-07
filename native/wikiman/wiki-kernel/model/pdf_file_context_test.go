//go:build fts5

package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/internal/pdfdevice"
)

func pdfFileFixture(t *testing.T) (*pdfPostprocessDocument, string, []byte, map[string][]byte) {
	t.Helper()
	d, before := pdfAssetsFixture(t)
	insertMarkdownRefText(t, d.m, d.headings[0].id, "File heading")
	p := pdfAssetTestPDF(t, PdfOutlineScheme+"://"+d.headings[0].id, pdfAssetOrigin+"/assets/same.pdf", "https://example.invalid/keep")
	raw := pipelinePDFBytes(t, p)
	target := filepath.Join(t.TempDir(), "selected 中文.pdf")
	if e := os.WriteFile(target, raw, 0640); e != nil {
		t.Fatal(e)
	}
	return d, target, raw, before
}
func assertPDFStageClean(t *testing.T, dir string) {
	t.Helper()
	entries, e := os.ReadDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range entries {
		if strings.HasPrefix(v.Name(), ".wiki-pdf-") {
			t.Fatal("owned stage left behind", v.Name())
		}
	}
}
func TestPDFFileFullProcessingAndReplacement(t *testing.T) {
	for _, embed := range []bool{false, true} {
		t.Run(map[bool]string{false: "relative", true: "embedded"}[embed], func(t *testing.T) {
			d, target, input, before := pdfFileFixture(t)
			oldInfo, _ := os.Stat(target)
			d.m.pdfWatermark = capturePDFWatermarkSnapshot(confExportWatermark("File watermark"), t.TempDir())
			globals := []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}
			if e := d.m.processPDFFile(d.tree.Root.ID, target, false, embed, true, pipelineDevice(t), defaultPDFFileOperations()); e != nil {
				t.Fatal(e)
			}
			raw, e := os.ReadFile(target)
			if e != nil || bytes.Equal(raw, input) {
				t.Fatal("file not replaced", e)
			}
			info, _ := os.Stat(target)
			if os.SameFile(oldInfo, info) || info.Mode().Perm() != 0640 {
				t.Fatal("replace/permission contract")
			}
			result := pipelineReadPDF(t, raw)
			bookmarks, e := pdfcpu.Bookmarks(result)
			if e != nil || len(bookmarks) != 1 || bookmarks[0].Title != "File heading" {
				t.Fatal(bookmarks, e)
			}
			requireWatermarkedPDF(t, result)
			if embed && string(pdfEmbeddedBytes(t, result)["same.pdf"]) != "global PDF" {
				t.Fatal("attachment bytes")
			}
			if !embed && !pdfAssetLinks(t, result)["assets/same.pdf"] {
				t.Fatal("relative URI")
			}
			if !reflect.DeepEqual(globals, []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}) {
				t.Fatal("parent PDF globals changed")
			}
			assertFileSourceUnchanged(t, before)
			assertPDFStageClean(t, filepath.Dir(target))
		})
	}
}
func TestPDFFileRejectsPathsAndSources(t *testing.T) {
	for _, kind := range []string{"relative", "traversal", "extension", "missing", "directory", "symlink", "data", "workspace", "app", "watermark", "oversized", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			d, target, raw, before := pdfFileFixture(t)
			switch kind {
			case "relative":
				target = "selected.pdf"
			case "traversal":
				target = filepath.Dir(target) + "/nested/../selected.pdf"
			case "extension":
				target = strings.TrimSuffix(target, ".pdf") + ".txt"
				os.WriteFile(target, raw, 0600)
			case "missing":
				target = filepath.Join(filepath.Dir(target), "missing.pdf")
			case "directory":
				target = filepath.Join(filepath.Dir(target), "directory.pdf")
				os.Mkdir(target, 0700)
			case "symlink":
				alias := filepath.Join(filepath.Dir(target), "alias.pdf")
				os.Symlink(target, alias)
				target = alias
			case "data":
				target = filepath.Join(d.m.read.binding.scope.commit.r.data, "assets", "same.pdf")
			case "workspace":
				target = filepath.Join(d.m.read.binding.scope.commit.r.workspace, "selected.pdf")
				os.WriteFile(target, raw, 0600)
			case "app":
				target = filepath.Join(d.m.htmlResources.work.path, "selected.pdf")
				os.WriteFile(target, raw, 0600)
			case "watermark":
				d.m.pdfWatermark = capturePDFWatermarkSnapshot(confExportWatermark(target), t.TempDir())
			case "oversized":
				f, e := os.OpenFile(target, os.O_WRONLY, 0)
				if e != nil {
					t.Fatal(e)
				}
				f.Truncate(pdfdevice.MaxPDF + 1)
				f.Close()
			case "readonly":
				os.Chmod(target, 0400)
			}
			called := false
			device := pdfDeviceBinding{execute: func(context.Context, pdfdevice.Request) ([]byte, error) { called = true; return nil, nil }}
			if e := d.m.processPDFFile(d.tree.Root.ID, target, false, false, false, device, defaultPDFFileOperations()); e == nil || PDFProcessCommitted(e) || called {
				t.Fatal("invalid source executed", e)
			}
			assertFileSourceUnchanged(t, before)
		})
	}
}
func TestPDFFileConcurrentChangesAndFailuresPreserveInput(t *testing.T) {
	for _, kind := range []string{"worker", "changed-inode", "same-metadata-edit", "parent-alias", "canceled", "closed", "write", "sync", "close", "rename", "short-stage"} {
		t.Run(kind, func(t *testing.T) {
			d, target, input, _ := pdfFileFixture(t)
			expected := input
			ops := defaultPDFFileOperations()
			base := pipelineDevice(t)
			calls := 0
			sentinel := errors.New("injected failure")
			selected := target
			if kind == "parent-alias" {
				alias := filepath.Join(t.TempDir(), "chosen")
				os.Symlink(filepath.Dir(target), alias)
				selected = filepath.Join(alias, filepath.Base(target))
			}
			device := base
			device.execute = func(ctx context.Context, r pdfdevice.Request) ([]byte, error) {
				calls++
				out, e := base.execute(ctx, r)
				if e != nil {
					return nil, e
				}
				if calls == 2 {
					switch kind {
					case "worker":
						return nil, sentinel
					case "changed-inode":
						os.Rename(target, target+".old")
						expected = []byte("external replacement")
						os.WriteFile(target, expected, 0640)
					case "same-metadata-edit":
						info, _ := os.Stat(target)
						expected = append([]byte(nil), input...)
						expected[len(expected)-1] ^= 1
						os.WriteFile(target, expected, 0640)
						os.Chtimes(target, info.ModTime(), info.ModTime())
					case "parent-alias":
						os.Remove(filepath.Dir(selected))
						os.Symlink(t.TempDir(), filepath.Dir(selected))
					case "canceled":
						cancelCtx, cancel := context.WithCancel(context.Background())
						cancel()
						d.m.read.binding.scope.ctx = cancelCtx
					case "closed":
						owner := d.m.read.binding.scope.commit.r.owner
						owner.repoMu.Lock()
						owner.repoClosed = true
						owner.repoMu.Unlock()
						t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
					}
				}
				return out, nil
			}
			switch kind {
			case "write":
				ops.write = func(*os.File, []byte) error { return sentinel }
			case "sync":
				ops.syncFile = func(*os.File) error { return sentinel }
			case "close":
				ops.closeFile = func(f *os.File) error { return errors.Join(f.Close(), sentinel) }
			case "rename":
				ops.rename = func(*os.Root, string, string) error { return sentinel }
			case "short-stage":
				ops.write = func(f *os.File, b []byte) error { _, e := f.Write(b[:len(b)/2]); return e }
			}
			e := d.m.processPDFFile(d.tree.Root.ID, selected, false, false, false, device, ops)
			if e == nil || PDFProcessCommitted(e) {
				t.Fatal("precommit failure lost", e)
			}
			got, _ := os.ReadFile(target)
			if !bytes.Equal(got, expected) {
				t.Fatal("failed operation overwrote original/current file")
			}
			assertPDFStageClean(t, filepath.Dir(target))
		})
	}
}
func TestPDFFileCommittedErrorsAndErrorChain(t *testing.T) {
	d, target, input, _ := pdfFileFixture(t)
	sentinel := errors.New("directory sync failed")
	ops := defaultPDFFileOperations()
	ops.syncDir = func(*os.Root) error { return sentinel }
	e := d.m.processPDFFile(d.tree.Root.ID, target, false, false, false, pipelineDevice(t), ops)
	if e == nil || !PDFProcessCommitted(e) || !errors.Is(e, sentinel) || !strings.Contains(e.Error(), "already updated") {
		t.Fatal("commit state not preserved", e)
	}
	got, _ := os.ReadFile(target)
	if bytes.Equal(got, input) {
		t.Fatal("committed rename did not happen")
	}
	pipelineReadPDF(t, got)
	assertPDFStageClean(t, filepath.Dir(target))
	joined := errors.Join(&PDFProcessError{Code: "first", Err: context.Canceled}, &PDFProcessError{Code: "cleanup", Committed: true, Err: sentinel})
	if !PDFProcessCommitted(joined) || !errors.Is(joined, context.Canceled) || !errors.Is(joined, sentinel) || PDFProcessCommitted(errors.New("plain")) {
		t.Fatal("joined error chain lost")
	}
}
func TestPDFFileCaptureAndPublicationWindowChecks(t *testing.T) {
	d, target, input, _ := pdfFileFixture(t)
	file, e := d.m.capturePDFFile(target)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { file.root.Close() })
	raw, e := file.read()
	if e != nil {
		t.Fatal(e)
	}
	file.digest = sha256.Sum256(raw)
	ops := defaultPDFFileOperations()
	original := ops.syncFile
	ops.syncFile = func(f *os.File) error {
		e := original(f)
		os.Remove(file.root.Name() + string(os.PathSeparator) + file.stage)
		os.WriteFile(filepath.Join(filepath.Dir(target), file.stage), []byte("foreign stage"), 0600)
		return e
	}
	if e = file.publish(input, ops); e == nil || file.committed {
		t.Fatal("foreign stage replaced destination", e)
	}
	foreign, _ := os.ReadFile(filepath.Join(filepath.Dir(target), file.stage))
	if string(foreign) != "foreign stage" {
		t.Fatal("foreign stage changed")
	}
	if e = file.clean(); e == nil {
		t.Fatal("foreign stage silently cleaned")
	}
	foreign, _ = os.ReadFile(filepath.Join(filepath.Dir(target), file.stage))
	if string(foreign) != "foreign stage" {
		t.Fatal("cleanup removed foreign stage")
	}
	if got, _ := os.ReadFile(target); !bytes.Equal(got, input) {
		t.Fatal("input overwritten")
	}
}

func TestPDFFilePublicContextAndCompatibilityWrapper(t *testing.T) {
	d, target, input, before := pdfFileFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := ProcessPDFContext(ctx, d.tree.Root.ID, target, false, false, false); !errors.Is(e, context.Canceled) || PDFProcessCommitted(e) {
		t.Fatal("request cancellation lost", e)
	}
	raw, _ := os.ReadFile(target)
	if !bytes.Equal(raw, input) {
		t.Fatal("canceled public call changed input")
	}
	if e := ProcessPDF(d.tree.Root.ID, target, false, false, false); e != nil {
		t.Fatal("public compatibility wrapper did not use isolated controller", e)
	}
	raw, _ = os.ReadFile(target)
	pipelineReadPDF(t, raw)
	if bytes.Equal(raw, input) {
		t.Fatal("public wrapper did not publish")
	}
	assertFileSourceUnchanged(t, before)
	assertPDFStageClean(t, filepath.Dir(target))
}
