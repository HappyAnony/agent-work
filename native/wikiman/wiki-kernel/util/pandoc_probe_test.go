package util

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExplicitPandocProbePrefixCancellationAndBoundedCapture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX synthetic executable fixture")
	}
	if valid, e := ValidatePandocBin(context.Background(), filepath.Join(t.TempDir(), "missing")); e != nil || valid {
		t.Fatal(valid, e)
	}
	file := filepath.Join(t.TempDir(), "tool")
	if e := os.WriteFile(file, []byte("#!/bin/sh\nprintf 'pandoc fixture\\n'\n"), 0700); e != nil {
		t.Fatal(e)
	}
	if valid, e := ValidatePandocBin(context.Background(), file); e != nil || !valid {
		t.Fatal(valid, e)
	}
	if e := os.WriteFile(file, []byte("#!/bin/sh\nexec sleep 10\n"), 0700); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	if valid, e := ValidatePandocBin(ctx, file); valid || !errors.Is(e, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatal(valid, e)
	}
	b := &pandocProbeOutput{}
	input := []byte(strings.Repeat("x", 128<<10))
	n, e := b.Write(input)
	if e != nil || n != len(input) || len(b.data) != 64<<10 {
		t.Fatal(n, e, len(b.data))
	}
}
