//go:build darwin

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/radovskyb/watcher"
	"wiki-kernel/internal/pollwatch"
	"wiki-kernel/util"
)

func TestLegacyWatchersInitializationAndActiveCallbackDrain(t *testing.T) {
	originalConf := Conf
	originalData, originalAssetsOpen, originalEmojisOpen := util.DataDir, openAssetsWatcher, openEmojisWatcher
	defer func() {
		_ = CloseWatchAssetsContext(context.Background())
		_ = CloseWatchEmojisContext(context.Background())
		Conf = originalConf
		util.DataDir, openAssetsWatcher, openEmojisWatcher = originalData, originalAssetsOpen, originalEmojisOpen
	}()
	Conf = &AppConf{}
	util.DataDir = t.TempDir()
	assets := filepath.Join(util.DataDir, "assets")
	emojis := filepath.Join(util.DataDir, "emojis")
	if err := os.MkdirAll(assets, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(emojis, 0700); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	openAssetsWatcher = func(ctx context.Context, folder string, _ time.Duration, _ func(context.Context, watcher.Event), report func(error)) (*pollwatch.Watch, error) {
		return pollwatch.Open(ctx, folder, 2*time.Millisecond, func(context.Context, watcher.Event) {
			once.Do(func() { close(entered) })
			<-release
		}, report)
	}
	if err := WatchAssetsContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := assetsWatcher
	cancelled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if err := WatchAssetsContext(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if assetsWatcher != first {
		t.Fatal("cancelled watcher start replaced the active owner")
	}
	if err := os.WriteFile(filepath.Join(assets, "active.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("watch callback did not enter")
	}
	deadline, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := CloseWatchAssetsContext(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("active callback reported drained", err)
	}
	if assetsWatcher == nil {
		t.Fatal("timed out watcher lost retry owner")
	}
	close(release)
	if err := CloseWatchAssetsContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if assetsWatcher != nil {
		t.Fatal("drained watcher retained")
	}

	injected := errors.New("injected emoji watcher initialization failure")
	openEmojisWatcher = func(context.Context, string, time.Duration, func(context.Context, watcher.Event), func(error)) (*pollwatch.Watch, error) {
		return nil, injected
	}
	if err := WatchEmojisContext(context.Background()); !errors.Is(err, injected) {
		t.Fatal(err)
	}
	if emojisWatcher != nil {
		t.Fatal("failed watcher initialization published an owner")
	}
}
