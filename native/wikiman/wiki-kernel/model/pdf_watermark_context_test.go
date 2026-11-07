//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func watermarkDocument(t *testing.T, value, description, appearance string) *pdfPostprocessDocument {
	t.Helper()
	m, box := markdownFixture(t)
	m.settings.PDFWatermarkStr = value
	m.settings.PDFWatermarkDesc = description
	m.pdfWatermark = capturePDFWatermarkSnapshot(m.settings, appearance)
	id := ast.NewNodeID()
	nativePreRenderDoc(t, m, box, "/"+id+".docj", footParagraph(ast.NewNodeID(), map[string]interface{}{"Type": "NodeText", "Data": "unchanged native source"}))
	d, e := m.preparePDFPostprocessDocument(id, false)
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func watermarkPNG(t *testing.T, filename string) {
	t.Helper()
	f, e := os.Create(filename)
	if e != nil {
		t.Fatal(e)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 240, A: 255})
		}
	}
	if e = png.Encode(f, img); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
func requireWatermarkedPDF(t *testing.T, pdf *pdfmodel.Context) {
	t.Helper()
	pdf = pdfBookmarkRoundTrip(t, pdf)
	if e := pdfcpu.DetectWatermarks(pdf); e != nil || !pdf.Watermarked {
		t.Fatal("watermark missing after actual PDF roundtrip", e)
	}
}
func TestPDFWatermarkTextImageAndNoGlobalMutation(t *testing.T) {
	for _, mode := range []string{"text", "image"} {
		t.Run(mode, func(t *testing.T) {
			beforeDir, beforeConfig, beforeVersion := font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr
			value := "OWNED WATERMARK"
			if mode == "image" {
				value = filepath.Join(t.TempDir(), "图 stamp.png")
				watermarkPNG(t, value)
			}
			d := watermarkDocument(t, value, "opacity:0.3,rotation:0", t.TempDir())
			var sourceBefore []byte
			if mode == "image" {
				sourceBefore, _ = os.ReadFile(value)
			}
			p := pdfTestContext(t)
			device := compatibilityPDFWatermarkDevice()
			parse := device.parse
			device.parse = func(spec pdfWatermarkSpec) (*pdfmodel.Watermark, error) {
				if spec.mode != mode {
					t.Fatal(spec.mode)
				}
				if mode == "image" && (spec.text != "" || len(spec.media) == 0) {
					t.Fatal("codec received source filename or no bytes")
				}
				return parse(spec)
			}
			apply := device.apply
			device.apply = func(pdf *pdfmodel.Context, wm *pdfmodel.Watermark) error {
				if !wm.OnTop {
					t.Fatal("watermark is not on top")
				}
				return apply(pdf, wm)
			}
			if e := d.watermark(p, true, device); e != nil {
				t.Fatal(e)
			}
			requireWatermarkedPDF(t, p)
			if beforeDir != font.UserFontDir || beforeConfig != pdfmodel.ConfigPath || beforeVersion != pdfmodel.VersionStr {
				t.Fatal("owned core changed PDF globals")
			}
			if mode == "image" {
				after, _ := os.ReadFile(value)
				if !bytes.Equal(sourceBefore, after) {
					t.Fatal("media source changed")
				}
			}
			if d.m.read.output != nil {
				t.Fatal("watermark port published output")
			}
		})
	}
}
func TestPDFWatermarkCapturedSettingsAndMode(t *testing.T) {
	m, box := markdownFixture(t)
	Conf.m.Lock()
	Conf.Export.PDFWatermarkStr = "CAPTURED"
	Conf.Export.PDFWatermarkDesc = "opacity:0.4"
	Conf.m.Unlock()
	captured, e := captureMarkdownExport(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	Conf.m.Lock()
	Conf.Export.PDFWatermarkStr = "CHANGED"
	Conf.Export.PDFWatermarkDesc = "invalid"
	Conf.m.Unlock()
	id := ast.NewNodeID()
	nativePreRenderDoc(t, captured, box, "/"+id+".docj", focusHeading(ast.NewNodeID(), 1, "native"))
	d, e := captured.preparePDFPostprocessDocument(id, false)
	if e != nil {
		t.Fatal(e)
	}
	device := compatibilityPDFWatermarkDevice()
	parse := device.parse
	seen := false
	device.parse = func(s pdfWatermarkSpec) (*pdfmodel.Watermark, error) {
		seen = true
		if s.text != "CAPTURED" || s.description != "opacity:0.4" {
			t.Fatal("settings followed global mutation", s)
		}
		return parse(s)
	}
	if e = d.watermark(pdfTestContext(t), true, device); e != nil || !seen {
		t.Fatal(e)
	}
	_ = m
	missing := filepath.Join(t.TempDir(), "not-created.png")
	settings := conf.Export{PDFWatermarkStr: missing}
	snapshot := capturePDFWatermarkSnapshot(settings, t.TempDir())
	watermarkPNG(t, missing)
	if snapshot.mode != "text" {
		t.Fatal("missing file changed original literal-text fallback")
	}
	called := false
	dev := pdfWatermarkDevice{parse: func(s pdfWatermarkSpec) (*pdfmodel.Watermark, error) {
		called = true
		if s.mode != "text" || s.text != missing {
			t.Fatal(s)
		}
		return &pdfmodel.Watermark{}, nil
	}, apply: func(*pdfmodel.Context, *pdfmodel.Watermark) error { return nil }}
	if e = applyCapturedPDFWatermark(pdfTestContext(t), snapshot, true, dev, func() error { return nil }); e != nil || !called {
		t.Fatal(e)
	}
}
func TestPDFWatermarkSourceIdentityAliasesAndLimits(t *testing.T) {
	for _, kind := range []string{"overwrite", "replace", "alias", "closed", "cancel", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "watermark.png")
			watermarkPNG(t, source)
			selected := source
			if kind == "alias" {
				selected = filepath.Join(t.TempDir(), "alias.png")
				os.Symlink(source, selected)
			}
			d := watermarkDocument(t, selected, "", t.TempDir())
			switch kind {
			case "overwrite":
				os.WriteFile(source, []byte("changed"), 0644)
			case "replace":
				os.Rename(source, source+".old")
				watermarkPNG(t, source)
			case "alias":
				os.Remove(selected)
				other := filepath.Join(t.TempDir(), "other.png")
				watermarkPNG(t, other)
				os.Symlink(other, selected)
			case "closed":
				owner := d.m.read.binding.scope.commit.r.owner
				owner.repoMu.Lock()
				owner.repoClosed = true
				owner.repoMu.Unlock()
				t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.m.read.binding.scope.ctx = ctx
			case "oversized":
				f, e := os.OpenFile(source, os.O_WRONLY, 0)
				if e != nil {
					t.Fatal(e)
				}
				f.Truncate(pdfWatermarkMediaLimit + 1)
				f.Close()
				d.m.pdfWatermark = capturePDFWatermarkSnapshot(d.m.settings, t.TempDir())
			}
			called := false
			dev := pdfWatermarkDevice{parse: func(pdfWatermarkSpec) (*pdfmodel.Watermark, error) { called = true; return &pdfmodel.Watermark{}, nil }, apply: func(*pdfmodel.Context, *pdfmodel.Watermark) error { return nil }}
			if e := d.watermark(pdfTestContext(t), true, dev); e == nil || called {
				t.Fatal("invalid source reached codec", e, called)
			}
		})
	}
	file := filepath.Join(t.TempDir(), "explicit.png")
	watermarkPNG(t, file)
	alias := filepath.Join(t.TempDir(), "alias.png")
	os.Symlink(file, alias)
	d := watermarkDocument(t, alias, "", t.TempDir())
	if e := d.watermark(pdfTestContext(t), true, compatibilityPDFWatermarkDevice()); e != nil {
		t.Fatal("captured explicit alias not supported", e)
	}
}
func TestPDFWatermarkFontSelectionAndFailures(t *testing.T) {
	for _, c := range []struct {
		desc     string
		names    []string
		fallback bool
		want     string
	}{{"opacity:0.3", nil, true, "fontname:" + pdfWatermarkDefaultFont}, {" fontname:ScopedFont , opacity:0.2", []string{"Corefonts:", "  ScopedFont (123 glyphs)"}, false, "fontname:ScopedFont"}, {"fontname:Unknown", []string{"Other"}, true, "fontname:" + pdfWatermarkDefaultFont}} {
		got, fallback := pdfWatermarkFontDescription(c.desc, c.names)
		if fallback != c.fallback || !strings.Contains(got, c.want) {
			t.Fatal(got, fallback)
		}
		again, _ := pdfWatermarkFontDescription(c.desc, c.names)
		if got != again {
			t.Fatal("unstable parameter ordering")
		}
	}
	d := watermarkDocument(t, "中文水印", "fontname:ScopedFont", t.TempDir())
	parsed := false
	dev := pdfWatermarkDevice{fonts: func() ([]string, error) { return []string{"ScopedFont (loaded)"}, nil }, parse: func(s pdfWatermarkSpec) (*pdfmodel.Watermark, error) {
		parsed = true
		if !strings.Contains(s.description, "fontname:ScopedFont") {
			t.Fatal(s.description)
		}
		return &pdfmodel.Watermark{}, nil
	}, apply: func(*pdfmodel.Context, *pdfmodel.Watermark) error { return nil }}
	if e := d.watermark(pdfTestContext(t), true, dev); e != nil || !parsed {
		t.Fatal("preloaded font unexpectedly needs install permission", e)
	}
	for _, kind := range []string{"unbound-fonts", "list", "install", "parse", "apply"} {
		t.Run(kind, func(t *testing.T) {
			appearance := t.TempDir()
			fontFile := filepath.Join(appearance, "fonts/LxgwWenKai-Lite-1.501", pdfWatermarkDefaultFont+".ttf")
			os.MkdirAll(filepath.Dir(fontFile), 0755)
			os.WriteFile(fontFile, []byte("test font bytes"), 0644)
			d := watermarkDocument(t, "中文", "", appearance)
			sentinel := errors.New("synthetic " + kind)
			dev := pdfWatermarkDevice{fonts: func() ([]string, error) { return nil, nil }, install: func(string, []byte) error { return nil }, parse: func(pdfWatermarkSpec) (*pdfmodel.Watermark, error) { return &pdfmodel.Watermark{}, nil }, apply: func(*pdfmodel.Context, *pdfmodel.Watermark) error { return nil }}
			switch kind {
			case "unbound-fonts":
				dev.fonts = nil
			case "list":
				dev.fonts = func() ([]string, error) { return nil, sentinel }
			case "install":
				dev.install = func(string, []byte) error { return sentinel }
			case "parse":
				dev.parse = func(pdfWatermarkSpec) (*pdfmodel.Watermark, error) { return nil, sentinel }
			case "apply":
				dev.apply = func(*pdfmodel.Context, *pdfmodel.Watermark) error { return sentinel }
			}
			e := d.watermark(pdfTestContext(t), true, dev)
			if e == nil || kind != "unbound-fonts" && !errors.Is(e, sentinel) {
				t.Fatal(e)
			}
		})
	}
}
func TestPDFWatermarkNoOpsAndUnboundExecution(t *testing.T) {
	guard := func() error { return nil }
	if e := applyCapturedPDFWatermark(nil, pdfWatermarkSnapshot{}, false, pdfWatermarkDevice{}, guard); e != nil {
		t.Fatal(e)
	}
	if e := applyCapturedPDFWatermark(nil, pdfWatermarkSnapshot{bound: true}, true, pdfWatermarkDevice{}, guard); e != nil {
		t.Fatal(e)
	}
	for _, s := range []pdfWatermarkSnapshot{{}, {bound: true, value: "enabled", mode: "text"}} {
		if e := applyCapturedPDFWatermark(pdfTestContext(t), s, true, pdfWatermarkDevice{}, guard); e == nil {
			t.Fatal("unbound execution accepted")
		}
	}
	if e := (&pdfPostprocessDocument{}).watermark(nil, true, pdfWatermarkDevice{}); e == nil {
		t.Fatal("unbound source accepted")
	}
	d := watermarkDocument(t, "test", "invalid parameters", t.TempDir())
	if e := d.watermark(pdfTestContext(t), true, compatibilityPDFWatermarkDevice()); e == nil {
		t.Fatal("parse failure hidden")
	}
	if e := applyCapturedPDFWatermark(nil, pdfWatermarkSnapshot{bound: true}, false, pdfWatermarkDevice{}, func() error { return context.Canceled }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestPDFWatermarkIsolatedCJKAndPDF(t *testing.T) {
	if os.Getenv("WIKI_WATERMARK_DEVICE_CHILD") != "1" {
		root := t.TempDir()
		before := []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}
		exe, e := os.Executable()
		if e != nil {
			t.Fatal(e)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestPDFWatermarkIsolatedCJKAndPDF$", "-test.v")
		cmd.Env = append(os.Environ(), "WIKI_WATERMARK_DEVICE_CHILD=1", "WIKI_WATERMARK_DEVICE_ROOT="+root, "HOME="+root)
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("isolated PDF device failed: %v\n%s", e, out)
		}
		if !reflect.DeepEqual(before, []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}) {
			t.Fatal("child changed parent globals")
		}
		if _, e = os.Stat(filepath.Join(root, "fonts", pdfWatermarkDefaultFont+".gob")); e != nil {
			t.Fatal("private CJK font was not installed", e)
		}
		return
	}
	root := os.Getenv("WIKI_WATERMARK_DEVICE_ROOT")
	if root == "" {
		t.Fatal("isolated device root required")
	}
	api.DisableConfigDir()
	font.UserFontDir = filepath.Join(root, "fonts")
	if e := os.Mkdir(font.UserFontDir, 0700); e != nil {
		t.Fatal(e)
	}
	appearance, e := filepath.Abs("../../wiki-app/appearance")
	if e != nil {
		t.Fatal(e)
	}
	d := watermarkDocument(t, "中文水印", "opacity:0.3", appearance)
	p := pdfTestContext(t)
	device := compatibilityPDFWatermarkDevice()
	if e = d.watermark(p, true, device); e != nil {
		t.Fatal(e)
	}
	requireWatermarkedPDF(t, p)
	source := filepath.Join(t.TempDir(), "input.PDF")
	raw, e := os.ReadFile("../testdata/parsertest.pdf")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(source, raw, 0644); e != nil {
		t.Fatal(e)
	}
	stamp := watermarkDocument(t, source, "opacity:0.2", appearance)
	p = pdfTestContext(t)
	if e = stamp.watermark(p, true, device); e != nil {
		t.Fatal(e)
	}
	requireWatermarkedPDF(t, p)
	after, _ := os.ReadFile(source)
	if !bytes.Equal(raw, after) {
		t.Fatal("PDF watermark source modified")
	}
}
