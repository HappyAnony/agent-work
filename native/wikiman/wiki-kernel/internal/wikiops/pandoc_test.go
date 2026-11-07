package wikiops

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPandocOptionalAndTrustedPaths(t *testing.T) {
	ctx := context.Background()
	if _, err := convertDocx(ctx, "<p>test</p>"); !errors.Is(err, ErrConverterUnavailable) {
		t.Fatal(err)
	}
	source := t.TempDir()
	for _, tool := range []string{"pandoc", filepath.Join(source, "missing"), filepath.Join(source, "inside")} {
		if filepath.IsAbs(tool) {
			_ = os.WriteFile(tool, []byte("not an executable"), 0700)
		}
		if _, err := convertDocx(WithPandoc(ctx, tool, source), "<p>test</p>"); !errors.Is(err, ErrConverterUnavailable) {
			t.Fatal(tool, err)
		}
	}
	if ErrorInfo(ErrConverterUnavailable).Type != "converter_unavailable" || ErrorInfo(ErrConversion).Type != "conversion_failed" {
		t.Fatal("untyped conversion errors")
	}
}

func TestPandocArchiveRejectsUnsafeExecutableEntries(t *testing.T) {
	for _, names := range [][]string{{"../pandoc"}, {"bin/pandoc", "bin/pandoc"}, {"bin/pandoc"}} {
		dir := t.TempDir()
		archive := filepath.Join(dir, "tool.zip")
		var data bytes.Buffer
		w := zip.NewWriter(&data)
		for _, name := range names {
			header := &zip.FileHeader{Name: name, Method: zip.Store}
			if len(names) == 1 && name == "bin/pandoc" {
				header.SetMode(os.ModeSymlink | 0700)
			}
			f, _ := w.CreateHeader(header)
			_, _ = f.Write([]byte("bad"))
		}
		_ = w.Close()
		_ = os.WriteFile(archive, data.Bytes(), 0600)
		if _, err := convertDocx(WithPandoc(context.Background(), archive, t.TempDir()), "<p>test</p>"); !errors.Is(err, ErrConverterUnavailable) {
			t.Fatal(names, err)
		}
	}
}

func TestPandocCancellationLimitsAndFailureDoNotWriteSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only; real target binary needs platform acceptance")
	}
	dir := t.TempDir()
	tool := filepath.Join(dir, "pandoc")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	root, m, id := setupPages(t)
	r := baseContent(id, "document.export")
	r.PageID = testPage
	r.DocID = "20261019090001-doc0001"
	r.Format = "docx"
	file := filepath.Join(root, "pages", testPage, "data", box, r.DocID+".docj")
	before, _ := os.ReadFile(file)
	ctx := WithPandoc(context.Background(), tool, root)
	if _, err := Content(ctx, m, r); !errors.Is(err, ErrConversion) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("conversion failure wrote source")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := convertDocx(cancelled, "<p>test</p>"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	output := &boundedOutput{limit: 3}
	if _, err := output.Write([]byte("four")); !errors.Is(err, ErrExportLimit) || output.Len() != 0 {
		t.Fatal(err)
	}
	// os/exec uses io.Copy; embedding bytes.Buffer would promote ReadFrom and
	// bypass Write's cap. Exercise a reader without a WriterTo shortcut too.
	if _, err := io.Copy(output, io.LimitReader(strings.NewReader("four"), 4)); !errors.Is(err, ErrExportLimit) || output.Len() != 0 {
		t.Fatal("copy bypassed output cap", err)
	}
	// Export format is a strict enum, and executable paths are never request fields.
	for _, format := range []string{"invalid-pdf-engine", "ast", "text"} {
		r.Format = format
		if r.Validate() == nil {
			t.Fatal(format)
		}
	}
	_, err := DecodeContentRequest(strings.NewReader(`{"protocolVersion":1,"action":"document.export","storeId":"x","pageId":"1760000000000000002","boxId":"20261019090000-box0001","docId":"20261019090001-doc0001","format":"docx","pandoc":"/tmp/evil"}`))
	if err == nil {
		t.Fatal("request selected converter")
	}
}

func TestPandocRefusesTempDirectoryInsideSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TMPDIR fixture is Unix-only")
	}
	source, tools := t.TempDir(), t.TempDir()
	tool := filepath.Join(tools, "pandoc")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", source)
	if _, err := convertDocx(WithPandoc(context.Background(), tool, source), "<p>test</p>"); !errors.Is(err, ErrConverterUnavailable) {
		t.Fatal(err)
	}
	files, err := os.ReadDir(source)
	if err != nil || len(files) != 0 {
		t.Fatal("conversion temp wrote into source", files, err)
	}
}

func TestPandocCancelsAnExecutingConverter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	dir, source := t.TempDir(), t.TempDir()
	tool := filepath.Join(dir, "pandoc ' with spaces")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(WithPandoc(context.Background(), tool, source), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := convertDocx(ctx, "<p>test</p>"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timed out converter was not reaped promptly")
	}
}
