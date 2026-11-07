package pdfdevice

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == WorkerArgument {
		if os.Getenv("WIKI_TEST_SECRET") != "" {
			os.Exit(3)
		}
		if e := Serve(os.Stdin, os.Stdout); e != nil {
			os.Stderr.WriteString("pdf_device_failed\n")
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fixture(t *testing.T) []byte {
	t.Helper()
	raw, e := os.ReadFile("../../testdata/parsertest.pdf")
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func testFont(t *testing.T) []byte {
	t.Helper()
	raw, e := os.ReadFile("../../../wiki-app/appearance/fonts/LxgwWenKai-Lite-1.501/" + DefaultFont + ".ttf")
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func inspect(t *testing.T, raw []byte, want bool) {
	t.Helper()
	c, e := api.ReadContext(bytes.NewReader(raw), &pdfmodel.Configuration{Reader15: true, ValidationMode: pdfmodel.ValidationRelaxed, Eol: types.EolLF})
	if e != nil {
		t.Fatal(e)
	}
	if e = api.ValidateContext(c); e != nil {
		t.Fatal(e)
	}
	if e = pdfcpu.DetectWatermarks(c); e != nil || c.Watermarked != want {
		t.Fatal("actual PDF watermark mismatch", e, c.Watermarked)
	}
}
func TestDeviceConcurrentPDFAndFontIsolation(t *testing.T) {
	input := fixture(t)
	before := append([]byte(nil), input...)
	ttf := testFont(t)
	var pngBytes bytes.Buffer
	if e := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 8, 8))); e != nil {
		t.Fatal(e)
	}
	globals := []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}
	fonts := font.UserFontNames()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	run, e := Bind(exe)
	if e != nil {
		t.Fatal(e)
	}
	cases := map[string]*Watermark{"latin": {Mode: "text", Text: "owned A", Description: "opacity:0.3"}, "image": {Mode: "image", Media: pngBytes.Bytes()}, "pdf": {Mode: "pdf", Media: input}, "cjk": {Mode: "text", Text: "中文水印", Fallback: ttf}}
	for name, w := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out, e := run(context.Background(), Request{Protocol: 1, PDF: input, Watermark: w, Version: name})
			if e != nil {
				t.Fatal(e)
			}
			inspect(t, out, true)
			if !bytes.Contains(out, []byte("Wiki "+name+" (pdfcpu ")) {
				t.Fatal("producer not owned by job")
			}
		})
	}
	t.Cleanup(func() {
		if !bytes.Equal(input, before) || !reflect.DeepEqual(globals, []string{font.UserFontDir, pdfmodel.ConfigPath, pdfmodel.VersionStr}) || !reflect.DeepEqual(fonts, font.UserFontNames()) {
			t.Error("parent PDF/font/global state changed")
		}
	})
}
func TestDeviceExplicitFontAndFailurePropagation(t *testing.T) {
	exe, _ := os.Executable()
	run, e := Bind(exe)
	if e != nil {
		t.Fatal(e)
	}
	raw := fixture(t)
	req := Request{Protocol: 1, PDF: raw, Fonts: []Font{{Name: DefaultFont, TTF: testFont(t)}}, Watermark: &Watermark{Mode: "text", Text: "中文", Description: "fontname:" + DefaultFont}}
	out, e := run(context.Background(), req)
	if e != nil {
		t.Fatal(e)
	}
	inspect(t, out, true)
	for _, tc := range []struct {
		name string
		r    Request
	}{{"bad-pdf", Request{Protocol: 1, PDF: []byte("broken")}}, {"missing-font", Request{Protocol: 1, PDF: raw, Watermark: &Watermark{Mode: "text", Text: "中文"}}}, {"bad-font", Request{Protocol: 1, PDF: raw, Fonts: []Font{{Name: "bad", TTF: []byte("bad")}}}}, {"bad-description", Request{Protocol: 1, PDF: raw, Watermark: &Watermark{Mode: "text", Text: "text", Description: "invalid-parameter:3"}}}} {
		t.Run(tc.name, func(t *testing.T) {
			out, e := run(context.Background(), tc.r)
			if e == nil || out != nil {
				t.Fatal("failed device returned output", e)
			}
		})
	}
	out, e = run(context.Background(), Request{Protocol: 1, PDF: raw})
	if e != nil {
		t.Fatal(e)
	}
	inspect(t, out, false)
}
func TestDeviceProtocolBoundsAndPrivateDirectory(t *testing.T) {
	base := Request{Protocol: 1, PDF: []byte("%PDF-")}
	for _, mutate := range []func(*Request){func(r *Request) { r.Protocol = 2 }, func(r *Request) { r.Version = "unsafe\n" }, func(r *Request) { r.Fonts = []Font{{Name: "../font", TTF: []byte("x")}} }, func(r *Request) { r.Fonts = []Font{{Name: "same", TTF: []byte("x")}, {Name: "same", TTF: []byte("x")}} }, func(r *Request) { r.Watermark = &Watermark{Mode: "file"} }, func(r *Request) { r.Watermark = &Watermark{Mode: "image", Text: "/private/file", Media: []byte("x")} }} {
		r := base
		mutate(&r)
		if Validate(r) == nil {
			t.Fatal("invalid request accepted")
		}
	}
	b := limitedBuffer{limit: 4}
	if _, e := b.Write([]byte("12345")); e == nil || b.Len() != 0 {
		t.Fatal("output overflow accepted")
	}
	exe, _ := os.Executable()
	for _, input := range []string{`{"Protocol":1,"PDF":"JVBERi0=","path":"/private/source"}`, `{"Protocol":1,"PDF":"JVBERi0="} {}`, `{broken`} {
		cmd := exec.Command(exe, WorkerArgument)
		cmd.Dir = t.TempDir()
		cmd.Stdin = strings.NewReader(input)
		out, e := cmd.Output()
		if e == nil || len(out) != 0 {
			t.Fatal("bad wire produced PDF", e)
		}
	}
	dir := t.TempDir()
	if e := os.Chmod(dir, 0755); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, WorkerArgument)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(`{}`)
	if e := cmd.Run(); e == nil {
		t.Fatal("public working directory accepted")
	}
}
func TestDeviceCancellationIdentityEnvironmentAndCleanup(t *testing.T) {
	t.Setenv("WIKI_TEST_SECRET", "do-not-forward")
	exe, _ := os.Executable()
	out, e := Run(context.Background(), exe, Request{Protocol: 1, PDF: fixture(t)})
	if e != nil {
		t.Fatal("parent environment leaked", e)
	}
	inspect(t, out, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out, e := Run(ctx, exe, Request{Protocol: 1, PDF: fixture(t)}); !errors.Is(e, context.Canceled) || out != nil {
		t.Fatal(e)
	}
	if _, e = Bind("relative"); e == nil {
		t.Fatal("relative executable accepted")
	}
	if os.PathSeparator == '\\' {
		return
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "device")
	if e = os.WriteFile(script, []byte("#!/bin/sh\nprintf '%%PDF-%s' \"$HOME\"\n"), 0700); e != nil {
		t.Fatal(e)
	}
	out, e = Run(context.Background(), script, Request{Protocol: 1, PDF: fixture(t)})
	if e != nil {
		t.Fatal(e)
	}
	private := strings.TrimPrefix(string(out), "%PDF-")
	if _, e = os.Stat(private); !os.IsNotExist(e) {
		t.Fatal("private directory leaked", e)
	}
	run, e := Bind(script)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(script, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0700); e != nil {
		t.Fatal(e)
	}
	if out, e = run(context.Background(), Request{Protocol: 1, PDF: fixture(t)}); e == nil || out != nil {
		t.Fatal("changed executable accepted")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	out, e = Run(ctx, script, Request{Protocol: 1, PDF: fixture(t)})
	if !errors.Is(e, context.DeadlineExceeded) || out != nil || time.Since(started) > 3*time.Second {
		t.Fatal("child not killed/joined", e)
	}
}
