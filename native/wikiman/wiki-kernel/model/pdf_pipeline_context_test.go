//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/pdfdevice"
)

func pipelineDevice(t *testing.T) pdfDeviceBinding {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	run, e := pdfdevice.Bind(exe)
	if e != nil {
		t.Fatal(e)
	}
	return pdfDeviceBinding{execute: run, version: "pipeline-owned"}
}
func pipelinePDFBytes(t *testing.T, p *pdfmodel.Context) []byte {
	t.Helper()
	var b bytes.Buffer
	if e := api.WriteContext(p, &b); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func pipelineReadPDF(t *testing.T, raw []byte) *pdfmodel.Context {
	t.Helper()
	p, e := api.ReadContext(bytes.NewReader(raw), &pdfmodel.Configuration{Reader15: true, ValidationMode: pdfmodel.ValidationRelaxed, Eol: types.EolLF})
	if e != nil {
		t.Fatal(e)
	}
	if e = api.ValidateContext(p); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestPDFPipelineFullIsolatedRoundTrip(t *testing.T) {
	for _, embed := range []bool{false, true} {
		t.Run(map[bool]string{false: "relative", true: "embed"}[embed], func(t *testing.T) {
			d, before := pdfAssetsFixture(t)
			if len(d.headings) != 1 {
				t.Fatal(d.headings)
			}
			insertMarkdownRefText(t, d.m, d.headings[0].id, "File heading")
			d.m.pdfWatermark = capturePDFWatermarkSnapshot(confExportWatermark("Pipeline stamp"), t.TempDir())
			external := "https://example.invalid/a%2Fb?q=%2520"
			p := pdfAssetTestPDF(t, PdfOutlineScheme+"://"+d.headings[0].id, pdfAssetOrigin+"/export/temp/assets/same.pdf?download=1#page=2", external)
			input := pipelinePDFBytes(t, p)
			inputBefore := append([]byte(nil), input...)
			globals := []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}
			fonts := font.UserFontNames()
			out, e := d.processPDFBytes(input, embed, true, pipelineDevice(t))
			if e != nil {
				t.Fatal(e)
			}
			result := pipelineReadPDF(t, out)
			bookmarks, e := pdfcpu.Bookmarks(result)
			if e != nil || len(bookmarks) != 1 || bookmarks[0].Title != "File heading" {
				t.Fatal("owned titles/outline", bookmarks, e)
			}
			if e = pdfcpu.DetectWatermarks(result); e != nil || !result.Watermarked {
				t.Fatal("watermark missing", e)
			}
			links := pdfAssetLinks(t, result)
			if !links[external] || len(result.PageAnnots[1][pdfmodel.AnnText].Map) != 1 {
				t.Fatal("external URI/note lost")
			}
			files := pdfEmbeddedBytes(t, result)
			if embed {
				if string(files["same.pdf"]) != "global PDF" {
					t.Fatal("global asset precedence", files)
				}
			} else if !links["assets/same.pdf#page=2"] || len(files) != 0 {
				t.Fatal(links, files)
			}
			if !bytes.Equal(input, inputBefore) || !reflect.DeepEqual(globals, []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}) || !reflect.DeepEqual(fonts, font.UserFontNames()) {
				t.Fatal("caller PDF/global state changed")
			}
			assertFileSourceUnchanged(t, before)
			if d.m.read.output != nil {
				t.Fatal("pipeline got publication")
			}
		})
	}
}
func TestPDFPipelineDirectoryMissingAndSelectedOnly(t *testing.T) {
	d, _ := pdfAssetsFixture(t)
	data := d.m.read.binding.scope.commit.r.data
	dir := filepath.Join(data, "assets", "tree.rtfd")
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "empty.txt"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	d.assets = append(d.assets, "assets/tree.rtfd", "assets/missing.txt")
	p := pdfAssetTestPDF(t, pdfAssetOrigin+"/assets/tree.rtfd/empty.txt", pdfAssetOrigin+"/assets/missing.txt")
	raw, e := d.processPDFBytes(pipelinePDFBytes(t, p), true, false, pipelineDevice(t))
	if e != nil {
		t.Fatal(e)
	}
	files := pdfEmbeddedBytes(t, pipelineReadPDF(t, raw))
	empty, ok := files["empty.txt"]
	if !ok || len(empty) != 0 || len(files) != 1 {
		t.Fatal("empty material/missing source policy", files)
	}
	if _, e = d.preparePDFDeviceAssets([]string{pdfAssetOrigin + "/assets/tree.rtfd"}, true); e == nil {
		t.Fatal("embedded directory accepted")
	}
	if _, e = d.preparePDFDeviceAssets([]string{pdfAssetOrigin + "/assets/unselected.txt"}, true); e == nil {
		t.Fatal("unselected local identifier accepted")
	}
	if _, e = d.preparePDFDeviceAssets([]string{pdfAssetOrigin + "/assets/%2e%2e/private"}, false); e == nil {
		t.Fatal("traversal accepted")
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	os.WriteFile(outside, []byte("private"), 0600)
	os.Symlink(outside, filepath.Join(dir, "linked.txt"))
	if _, e = d.preparePDFDeviceAssets([]string{pdfAssetOrigin + "/assets/tree.rtfd/linked.txt"}, true); e == nil {
		t.Fatal("source link accepted")
	}
}
func TestPDFPipelineFailureDiscardsAndNoSecondPhase(t *testing.T) {
	for _, kind := range []string{"inspect-error", "bad-inspection", "closed", "cancel", "sql", "final-error"} {
		t.Run(kind, func(t *testing.T) {
			d, _ := pdfAssetsFixture(t)
			calls := 0
			sentinel := errors.New("device failed")
			device := pdfDeviceBinding{execute: func(ctx context.Context, r pdfdevice.Request) ([]byte, error) {
				calls++
				if calls == 1 {
					switch kind {
					case "inspect-error":
						return []byte("partial"), sentinel
					case "bad-inspection":
						return []byte(`{"URIs":[],"path":"private"}`), nil
					case "closed":
						owner := d.m.read.binding.scope.commit.r.owner
						owner.repoMu.Lock()
						owner.repoClosed = true
						owner.repoMu.Unlock()
						t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
					}
					return []byte(`{"URIs":[]}`), nil
				}
				return []byte("partial"), sentinel
			}}
			if kind == "sql" {
				if e := d.m.read.binding.scope.commit.r.main.DB.Close(); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "cancel" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.m.read.binding.scope.ctx = ctx
			}
			out, e := d.processPDFBytes([]byte("%PDF-input"), true, false, device)
			if e == nil || out != nil {
				t.Fatal("failed pipeline produced output", e)
			}
			want := 1
			if kind == "cancel" || kind == "sql" {
				want = 0
			}
			if kind == "final-error" {
				want = 2
			}
			if calls != want {
				t.Fatal("wrong phase count", calls, want)
			}
		})
	}
}

func confExportWatermark(text string) conf.Export {
	return conf.Export{PDFWatermarkStr: text, PDFWatermarkDesc: "opacity:0.3"}
}
