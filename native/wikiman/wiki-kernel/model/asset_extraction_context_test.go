//go:build fts5

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/assetextract"
)

func TestAssetExtractionScanOriginalTextAndErrors(t *testing.T) {
	root := t.TempDir()
	if e := os.WriteFile(filepath.Join(root, "note.md"), []byte("original\ntext"), 0600); e != nil {
		t.Fatal(e)
	}
	exe, _ := os.Executable()
	run, e := assetextract.Bind(exe, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	env := assetExtractionEnvironment{run: run, pdfMax: assetextract.MaxInput}
	got, e := NewAssetsSearcher().scanWithExtraction(context.Background(), root, nil, env)
	if e != nil || len(got) != 1 || got[0].Path != "assets/note.md" || got[0].Content != "original\ntext" {
		t.Fatal(got, e)
	}
	if e = os.WriteFile(filepath.Join(root, "bad.docx"), []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = NewAssetsSearcher().scanWithExtraction(context.Background(), root, nil, env); e == nil {
		t.Fatal("conversion failure became successful empty index")
	}
	if e = os.Remove(filepath.Join(root, "bad.docx")); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(filepath.Join(t.TempDir(), "foreign.txt"), filepath.Join(root, "foreign.txt")); e != nil {
		t.Fatal(e)
	}
	if _, e = NewAssetsSearcher().scanWithExtraction(context.Background(), root, nil, env); e == nil {
		t.Fatal("outside resource accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = NewAssetsSearcher().scanWithExtraction(ctx, root, nil, env); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
