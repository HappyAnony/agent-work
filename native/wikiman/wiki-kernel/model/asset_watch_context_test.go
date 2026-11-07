//go:build fts5

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/cache"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/util"
)

func TestAssetWatchOwnedIndexCacheAndThumbnail(t *testing.T) {
	_, _, _, _, _ = referenceModelFixture(t)
	w, e := captureAssetWatchContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	file := archiveWrite(t, w.base.data, "assets/watcher.txt", []byte("owned watcher content"))
	if e = w.dispatch(context.Background(), assetflow.Event{Path: file, Written: true}); e != nil {
		t.Fatal(e)
	}
	if cache.GetAssets()["assets/watcher.txt"] == nil {
		t.Fatal("cache not refreshed")
	}
	var count int
	if e = w.queue.DB.QueryRow("SELECT count(*) FROM asset_contents_fts_case_insensitive WHERE path=? AND content LIKE ?", "assets/watcher.txt", "%owned watcher content%").Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	if e = os.Remove(file); e != nil {
		t.Fatal(e)
	}
	if e = w.dispatch(context.Background(), assetflow.Event{Path: file, Removed: true}); e != nil {
		t.Fatal(e)
	}
	if e = w.queue.DB.QueryRow("SELECT count(*) FROM asset_contents_fts_case_insensitive WHERE path=?", "assets/watcher.txt").Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
	image := archiveWrite(t, w.base.data, "assets/image.png", []byte("image"))
	thumbnail := archiveWrite(t, w.base.tempDir, "thumbnails/assets/image.png", []byte("old thumbnail"))
	if e = w.dispatch(context.Background(), assetflow.Event{Path: image, Removed: true}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(thumbnail); !os.IsNotExist(e) {
		t.Fatal("thumbnail retained", e)
	}
	if e = w.dispatch(context.Background(), assetflow.Event{Path: filepath.Join(t.TempDir(), "outside.txt")}); e == nil {
		t.Fatal("outside event admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = w.dispatch(ctx, assetflow.Event{Path: file}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	old := util.DataDir
	util.DataDir = t.TempDir()
	e = w.dispatch(context.Background(), assetflow.Event{Path: file})
	util.DataDir = old
	if e == nil {
		t.Fatal("stale owner admitted")
	}
}
