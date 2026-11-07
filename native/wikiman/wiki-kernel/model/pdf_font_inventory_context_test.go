//go:build fts5

package model

import (
	"bytes"
	"errors"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wiki-kernel/conf"
)

func legacyFontCache(t *testing.T, dir, name string) []byte {
	t.Helper()
	ttf, e := os.ReadFile("../../wiki-app/appearance/fonts/LxgwWenKai-Lite-1.501/LXGWWenKaiLite-Regular.ttf")
	if e != nil {
		t.Fatal(e)
	}
	if e = font.InstallFontFromBytes(dir, name, ttf); e != nil {
		t.Fatal(e)
	}
	actual := filepath.Join(dir, "LXGWWenKaiLite-Regular.gob")
	if name != "LXGWWenKaiLite-Regular" {
		if e = os.Rename(actual, filepath.Join(dir, name+".gob")); e != nil {
			t.Fatal(e)
		}
	}
	raw, e := os.ReadFile(filepath.Join(dir, name+".gob"))
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestPDFFontInventorySelectedCacheUsesIsolatedDecoder(t *testing.T) {
	dir := t.TempDir()
	name := "旧 Font.1"
	raw := legacyFontCache(t, dir, name)
	os.WriteFile(filepath.Join(dir, "bad.gob"), []byte("broken but unselected"), 0600)
	inventory := capturePDFFontInventory(dir)
	if inventory.err != nil {
		t.Fatal(inventory.err)
	}
	d, _ := pdfAssetsFixture(t)
	d.m.pdfFonts = inventory
	d.m.pdfWatermark = capturePDFWatermarkSnapshot(conf.Export{PDFWatermarkStr: "中文", PDFWatermarkDesc: "fontname:" + name + ",opacity:0.3"}, t.TempDir())
	d.m.pdfWatermark.fallback.err = errors.New("fallback must not be read")
	before := []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}
	fonts := font.UserFontNames()
	input := pipelinePDFBytes(t, pdfTestContext(t))
	for _, text := range []string{"中文", "Latin custom font"} {
		d.m.pdfWatermark.value = text
		output, e := d.processPDFBytes(input, false, true, pipelineDevice(t))
		if e != nil {
			t.Fatal(text, e)
		}
		requireWatermarkedPDF(t, pipelineReadPDF(t, output))
	}

	after, _ := os.ReadFile(filepath.Join(dir, name+".gob"))
	if !bytes.Equal(raw, after) || !reflect.DeepEqual(before, []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}) || !reflect.DeepEqual(fonts, font.UserFontNames()) {
		t.Fatal("old cache/global state changed")
	}
	legacyFontCache(t, dir, "late")
	if _, ok, e := inventory.selected("late", d.guard); e != nil || ok {
		t.Fatal("font inventory followed later additions", e)
	}
}
func TestPDFFontInventorySourceIdentityAndDeferredErrors(t *testing.T) {
	for _, kind := range []string{"overwrite", "root-replace", "link", "oversized", "bad-cache"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "Selected.gob")
			os.WriteFile(file, []byte("original"), 0600)
			if kind == "link" {
				os.Remove(file)
				outside := filepath.Join(t.TempDir(), "other.gob")
				os.WriteFile(outside, []byte("outside"), 0600)
				os.Symlink(outside, file)
			}
			if kind == "oversized" {
				f, e := os.OpenFile(file, os.O_WRONLY, 0600)
				if e != nil {
					t.Fatal(e)
				}
				f.Truncate(pdfWatermarkFontLimit + 1)
				f.Close()
			}
			captured := capturePDFFontInventory(dir)
			if kind == "overwrite" {
				os.WriteFile(file, []byte("changed cache"), 0600)
			}
			if kind == "root-replace" {
				os.Rename(dir, dir+"-old")
				t.Cleanup(func() { os.RemoveAll(dir + "-old") })
				os.Mkdir(dir, 0700)
				os.WriteFile(file, []byte("replacement"), 0600)
			}
			d, _ := pdfAssetsFixture(t)
			d.m.pdfFonts = captured
			d.m.pdfWatermark = capturePDFWatermarkSnapshot(conf.Export{PDFWatermarkStr: "中文", PDFWatermarkDesc: "fontname:Selected"}, t.TempDir())
			if out, e := d.watermarkBytes(pipelinePDFBytes(t, pdfTestContext(t)), true, pipelineDevice(t)); e == nil || out != nil {
				t.Fatal("invalid cache accepted", e)
			}
			// A broken, unused font inventory never blocks a disabled/Latin watermark.
			d.m.pdfFonts.err = errors.New("inventory unavailable")
			d.m.pdfWatermark = capturePDFWatermarkSnapshot(conf.Export{PDFWatermarkStr: "Latin"}, t.TempDir())
			if _, e := d.watermarkBytes(pipelinePDFBytes(t, pdfTestContext(t)), true, pipelineDevice(t)); e != nil {
				t.Fatal("unused inventory blocked export", e)
			}
		})
	}
}
