//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/pdfdevice"
)

func TestPDFDeviceCapturedMaterialAndExplicitBinding(t *testing.T) {
	file := filepath.Join(t.TempDir(), "stamp.png")
	watermarkPNG(t, file)
	before, _ := os.ReadFile(file)
	d := watermarkDocument(t, file, "opacity:0.2", t.TempDir())
	raw := []byte("%PDF-private-input")
	count := 0
	device := pdfDeviceBinding{version: "captured", fonts: []pdfdevice.Font{{Name: "custom", TTF: []byte("font bytes")}}, execute: func(ctx context.Context, r pdfdevice.Request) ([]byte, error) {
		count++
		if r.Version != "captured" || r.Protocol != 1 || r.Watermark.Mode != "image" || r.Watermark.Text != "" || !bytes.Equal(r.Watermark.Media, before) || !bytes.Equal(r.PDF, raw) {
			t.Fatal("wrong owned job")
		}
		r.PDF[0] = '!'
		r.Fonts[0].TTF[0] = '!'
		return []byte("%PDF-output"), nil
	}}
	out, e := d.watermarkBytes(raw, true, device)
	if e != nil || string(out) != "%PDF-output" || count != 1 {
		t.Fatal(e)
	}
	if string(raw) != "%PDF-private-input" || string(device.fonts[0].TTF) != "font bytes" {
		t.Fatal("executor borrowed caller slices")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("source modified")
	}
	if _, e = d.watermarkBytes(raw, true, pdfDeviceBinding{}); e == nil {
		t.Fatal("implicit global fallback")
	}
	if d.m.read.output != nil {
		t.Fatal("device got file publication")
	}
}
func TestPDFDeviceCapturedFontAndNoOp(t *testing.T) {
	appearance, _ := filepath.Abs("../../wiki-app/appearance")
	d := watermarkDocument(t, "中文", "", appearance)
	raw := []byte("%PDF-input")
	device := pdfDeviceBinding{execute: func(ctx context.Context, r pdfdevice.Request) ([]byte, error) {
		if r.Watermark == nil || len(r.Watermark.Fallback) == 0 {
			t.Fatal("fallback font not captured")
		}
		return []byte("%PDF-output"), nil
	}}
	if _, e := d.watermarkBytes(raw, true, device); e != nil {
		t.Fatal(e)
	}
	device.execute = func(ctx context.Context, r pdfdevice.Request) ([]byte, error) {
		if r.Watermark != nil {
			t.Fatal("disabled watermark material read")
		}
		return []byte("%PDF-output"), nil
	}
	d.m.pdfWatermark.fallback.err = errors.New("unreadable")
	if _, e := d.watermarkBytes(raw, false, device); e != nil {
		t.Fatal(e)
	}
	d.m.pdfWatermark.description = "fontname:Selected"
	device.fonts = []pdfdevice.Font{{Name: "Selected", TTF: []byte("explicit")}}
	device.execute = func(ctx context.Context, r pdfdevice.Request) ([]byte, error) {
		if len(r.Watermark.Fallback) != 0 || len(r.Fonts) != 1 {
			t.Fatal("ambient font fallback")
		}
		return []byte("%PDF-output"), nil
	}
	if _, e := d.watermarkBytes(raw, true, device); e != nil {
		t.Fatal(e)
	}
}
func TestPDFDeviceRejectsChangedSourceAndDiscardsFailedOutput(t *testing.T) {
	f := filepath.Join(t.TempDir(), "image.png")
	watermarkPNG(t, f)
	d := watermarkDocument(t, f, "", t.TempDir())
	called := false
	device := pdfDeviceBinding{execute: func(context.Context, pdfdevice.Request) ([]byte, error) { called = true; return nil, nil }}
	if e := os.WriteFile(f, []byte("changed"), 0600); e != nil {
		t.Fatal(e)
	}
	if out, e := d.watermarkBytes([]byte("%PDF-input"), true, device); e == nil || out != nil || called {
		t.Fatal("changed source executed")
	}
	d = watermarkDocument(t, "Latin", "", t.TempDir())
	sentinel := errors.New("worker failed")
	device.execute = func(context.Context, pdfdevice.Request) ([]byte, error) { return []byte("partial"), sentinel }
	if out, e := d.watermarkBytes([]byte("%PDF-input"), true, device); !errors.Is(e, sentinel) || out != nil {
		t.Fatal(e)
	}
	device.execute = func(context.Context, pdfdevice.Request) ([]byte, error) {
		owner := d.m.read.binding.scope.commit.r.owner
		owner.repoMu.Lock()
		owner.repoClosed = true
		owner.repoMu.Unlock()
		t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
		return []byte("%PDF-output"), nil
	}
	if out, e := d.watermarkBytes([]byte("%PDF-input"), true, device); e == nil || out != nil {
		t.Fatal("closed owner published output", e)
	}
}
