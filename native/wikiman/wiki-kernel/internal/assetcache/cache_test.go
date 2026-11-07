package assetcache

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
)

func TestOriginalAssetFilteringNamesCopiesAndFailure(t *testing.T) {
	ctx := context.Background()
	var c Cache
	files := fstest.MapFS{"photo-20261022000000-aaaaaaa.png": {Data: []byte("a")}, "nested/b.png": {Data: []byte("b")}, ".hidden/a": {Data: []byte("x")}, ".dot": {Data: []byte("x")}, "copy.sya": {Data: []byte("x")}, "flagged": {Data: []byte("x")}}
	if err := c.Load(ctx, files, func(p string) bool { return p == "flagged" }); err != nil {
		t.Fatal(err)
	}
	snap := c.Snapshot()
	if len(snap) != 2 || snap["assets/photo-20261022000000-aaaaaaa.png"].HName != "photo.png" {
		t.Fatal(snap)
	}
	delete(snap, "assets/nested/b.png")
	if !c.Exists("assets/nested/b.png") {
		t.Fatal("snapshot aliases owner")
	}
	c.SetHash("hash", "assets/nested/b.png")
	v, ok := c.GetHash("hash", func(string) bool { return true })
	if !ok || v.Path != "assets/nested/b.png" {
		t.Fatal(v, ok)
	}
	v.Path = "mutated"
	v, ok = c.GetHash("hash", func(string) bool { return true })
	if !ok || v.Path == "mutated" {
		t.Fatal(v)
	}
	if _, ok = c.GetHash("hash", func(string) bool { return false }); ok {
		t.Fatal("missing hash path returned")
	}
	bad := fstest.MapFS{"link": {Mode: fs.ModeSymlink}}
	if err := c.Load(ctx, bad, nil); err == nil {
		t.Fatal("symlink accepted")
	}
	if len(c.Snapshot()) != 2 {
		t.Fatal("failed scan replaced valid cache")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := c.Load(cancelled, files, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	c.Remove("assets/nested/b.png")
	if c.Exists("assets/nested/b.png") {
		t.Fatal("remove failed")
	}
	c.Close()
	if err := c.Load(ctx, files, nil); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
func TestAssetOwnersAndConcurrentCopies(t *testing.T) {
	var a, b Cache
	ctx := context.Background()
	if err := a.Load(ctx, fstest.MapFS{"a.png": {Data: []byte("a")}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(b.Snapshot()) != 0 {
		t.Fatal("shared asset state")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				a.SetHash("h", "assets/a.png")
				a.GetHash("h", func(string) bool { return true })
				a.Snapshot()
				a.RemoveHash("h")
			}
		}()
	}
	wg.Wait()
}

func TestAssetManifestFailuresPreservePublishedSnapshot(t *testing.T) {
	for _, kind := range []string{"incomplete", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			var c Cache
			if err := c.Load(context.Background(), fstest.MapFS{"stable.png": {Data: []byte("stable")}}, nil); err != nil {
				t.Fatal(err)
			}
			c.SetHash("stable-hash", "assets/stable.png")
			files := fstest.MapFS{"a.png": {Data: []byte("first")}, "zdir/b.png": {Data: []byte("second")}}
			primary := errors.New("asset enumeration failed")
			op := loadIO{readDir: func(root fs.FS, name string) ([]fs.DirEntry, error) {
				if name != "zdir" {
					return fs.ReadDir(root, name)
				}
				if kind == "incomplete" {
					return nil, errors.Join(fs.ErrNotExist, primary)
				}
				files["a.png"] = &fstest.MapFile{Data: []byte("replacement")}
				return fs.ReadDir(root, name)
			}}
			err := c.loadWithIO(context.Background(), files, nil, op)
			if kind == "incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, fs.ErrNotExist) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrSourceChanged) {
				t.Fatal(err)
			}
			snapshot := c.Snapshot()
			if len(snapshot) != 1 || !c.Exists("assets/stable.png") || c.Exists("assets/a.png") {
				t.Fatal("failed manifest replaced published assets", snapshot)
			}
			if got, ok := c.GetHash("stable-hash", func(string) bool { return true }); !ok || got.Path != "assets/stable.png" {
				t.Fatal("failed manifest cleared hash cache", got, ok)
			}
		})
	}
}

func TestAssetManifestProductionIdentityReplacement(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(rootDir, "zdir"), 0700); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(rootDir, "a.png")
	if err := os.WriteFile(original, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "zdir", "b.png"), []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	fileInfo, err := os.Stat(original)
	if err != nil {
		t.Fatal(err)
	}
	rootInfo, err := os.Stat(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(filepath.Dir(rootDir), "asset-replacement")
	if err = os.WriteFile(replacement, []byte("other"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(replacement) })
	if err = os.Chtimes(replacement, fileInfo.ModTime(), fileInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(filepath.Dir(rootDir), "asset-original")
	t.Cleanup(func() { _ = os.Remove(old) })
	root := os.DirFS(rootDir)
	op := loadIO{readDir: func(source fs.FS, name string) ([]fs.DirEntry, error) {
		if name == "zdir" {
			if renameErr := os.Rename(original, old); renameErr != nil {
				t.Fatal(renameErr)
			}
			if renameErr := os.Rename(replacement, original); renameErr != nil {
				t.Fatal(renameErr)
			}
			if timeErr := os.Chtimes(rootDir, rootInfo.ModTime(), rootInfo.ModTime()); timeErr != nil {
				t.Fatal(timeErr)
			}
		}
		return fs.ReadDir(source, name)
	}}
	var c Cache
	if err = c.loadWithIO(context.Background(), root, nil, op); !errors.Is(err, ErrSourceChanged) || len(c.Snapshot()) != 0 {
		t.Fatal(err, c.Snapshot())
	}
}
