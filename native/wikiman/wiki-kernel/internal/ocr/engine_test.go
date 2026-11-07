package ocr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fakeBinary(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fake tesseract")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0700); err != nil {
		t.Fatal(err)
	}
	return p
}

const fakeTesseract = `case "$1" in
 --version) printf 'tesseract fixture-1\n'; exit 0;;
 --list-langs) printf 'List of available languages (4):\neng\nchi_sim\nfra\nother\n'; exit 0;;
esac
IFS= read -r content < "$3"
printf 'level\ttext\n5\t%s\n' "$content"
`

func TestExplicitProbeOriginalLanguagesTSVAndInputCopy(t *testing.T) {
	opt := Defaults()
	opt.Candidates = []string{fakeBinary(t, fakeTesseract)}
	opt.Languages = []string{"chi_sim", "eng"}
	engine, err := Open(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	opt.Languages[0] = "mutated"
	if !reflect.DeepEqual(engine.Languages, []string{"eng", "chi_sim"}) || engine.Version != "fixture-1" || engine.Signature == "" {
		t.Fatal(engine)
	}
	rows, err := engine.Recognize(context.Background(), "assets/a.png", []byte("hello OCR"))
	if err != nil || Text(rows) != " hello OCR" {
		t.Fatal(rows, Text(rows), err)
	}
	if rows, err = engine.Recognize(context.Background(), "assets/a.pdf", []byte("x")); err != nil || rows != nil {
		t.Fatal(rows, err)
	}
	if rows, err = engine.Recognize(context.Background(), "assets/a.png", make([]byte, opt.MaxSize+1)); err != nil || rows != nil {
		t.Fatal(err)
	}
	disabled := Defaults()
	disabled.Enabled = false
	disabled.Candidates = []string{"/not-present"}
	off, err := Open(context.Background(), disabled)
	if err != nil || off.Enabled {
		t.Fatal(off, err)
	}
	if got := Text(ParseTSV("level\ttext\r\n5\t中\r\n5\t文\r\n5\t word\n")); got != " 中文word" {
		t.Fatal(got)
	}
}
func TestOCRTimeoutCancellationAndBoundedOutput(t *testing.T) {
	opt := Defaults()
	opt.Timeout = 7 * time.Second
	opt.Candidates = []string{fakeBinary(t, `case "$1" in
 --version) printf 'tesseract fixture-1\n';exit 0;;
 --list-langs) printf 'List:\neng\n';exit 0;;
esac
exec sleep 30
`)}
	engine, err := Open(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	engine.options.Timeout = 50 * time.Millisecond // Probe startup has a separate test budget.
	start := time.Now()
	_, err = engine.Recognize(context.Background(), "a.png", []byte("x"))
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatal(err, time.Since(start))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = engine.Recognize(ctx, "a.png", []byte("x")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var out boundedOutput
	if _, err = out.Write([]byte(strings.Repeat("x", (16<<20)+1))); err == nil {
		t.Fatal("output limit ignored")
	}
}
