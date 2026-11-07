//go:build fts5

package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/internal/assetextract"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == assetextract.WorkerArgument {
		var q assetextract.Request
		if e := json.NewDecoder(os.Stdin).Decode(&q); e != nil {
			os.Exit(2)
		}
		if string(q.Data) == "test-native-cancel" {
			os.WriteFile("started", []byte("started"), 0600)
			time.Sleep(time.Hour)
			os.Exit(2)
		}
		raw, _ := json.Marshal(q)
		if e := assetextract.Serve(bytes.NewReader(raw), os.Stdout); e != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestNativeAssetExtractionDrainJoinsConverter(t *testing.T) {
	root := infoSource(t)
	if e := os.WriteFile(filepath.Join(root, "pages", pageA, "data", "assets", "slow.docx"), []byte("test-native-cancel"), 0600); e != nil {
		t.Fatal(e)
	}
	m := open(t, root, lifecycleOptions(root))
	l := getDocLease(t, m, pageA)
	store := l.StoreID()
	temp := l.workspace.paths.TempDir
	done := make(chan error, 1)
	go func() { _, e := l.ExtractNativeAsset(context.Background(), "assets/slow.docx"); done <- e }()
	deadline := time.Now().Add(10 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob(filepath.Join(temp, "asset-extract-*", "started"))
		if len(matches) > 0 {
			started = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !started {
		t.Fatal("converter did not start")
	}
	l.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e := m.DrainStore(ctx, store); e != nil {
		t.Fatal(e)
	}
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if matches, _ := filepath.Glob(filepath.Join(temp, "asset-extract-*")); len(matches) != 0 {
		t.Fatal("converter cleanup survived drain", matches)
	}
}

func TestNativeAssetExtractionOwnerIsolationAndNoPublication(t *testing.T) {
	root := infoSource(t)
	for _, page := range []string{pageA, pageB} {
		if e := os.WriteFile(filepath.Join(root, "pages", page, "data", "assets", "extract.txt"), []byte(page), 0600); e != nil {
			t.Fatal(e)
		}
	}
	m := open(t, root, lifecycleOptions(root))
	before := digest(t, root)
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		got, e := l.ExtractNativeAsset(context.Background(), "assets/extract.txt")
		if e != nil || got.Content != page || got.SourceHash != sha256.Sum256([]byte(page)) {
			t.Fatal(got, e)
		}
		if _, e = l.ExtractNativeAsset(context.Background(), "assets/../../escape.txt"); e == nil {
			t.Fatal("outside resource accepted")
		}
	}
	if digest(t, root) != before {
		t.Fatal("extraction changed source")
	}
}
