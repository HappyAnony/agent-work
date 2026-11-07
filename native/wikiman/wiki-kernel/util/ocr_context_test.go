package util

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyOCRWaitCancellationAndCorruptSeedPreserved(t *testing.T) {
	oldData, oldTexts := DataDir, assetsTexts
	defer func() { DataDir = oldData; assetsTexts = oldTexts }()
	DataDir = t.TempDir()
	assetsTexts = map[string]string{"assets/old.png": "keep"}
	dir := filepath.Join(DataDir, "assets")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(dir, "ocr-texts.json")
	if e := os.WriteFile(file, []byte("broken json"), 0600); e != nil {
		t.Fatal(e)
	}
	LoadAssetsTexts()
	if got := GetAssetText("assets/old.png"); got != "keep" {
		t.Fatal(got)
	}
	raw, e := os.ReadFile(file)
	if e != nil || string(raw) != "broken json" {
		t.Fatal(string(raw), e)
	}
	was := tesseractInited.Load()
	tesseractInited.Store(false)
	defer tesseractInited.Store(was)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if e := WaitForTesseractInitContext(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
}
