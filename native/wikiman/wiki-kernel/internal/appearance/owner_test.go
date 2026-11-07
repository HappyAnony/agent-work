package appearance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func fixture(t *testing.T) (Options, *conf.Appearance) {
	t.Helper()
	base := t.TempDir()
	opts := Options{Bundled: filepath.Join(base, "bundle"), Destination: filepath.Join(base, "installed")}
	opts.Themes = filepath.Join(opts.Destination, "themes")
	opts.Icons = filepath.Join(opts.Destination, "icons")
	write := func(root, name, value string) {
		t.Helper()
		file := filepath.Join(root, name)
		if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(file, []byte(value), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write(opts.Bundled, "themes/daylight/theme.json", `{"version":"1","modes":["light"]}`)
	write(opts.Bundled, "themes/midnight/theme.json", `{"version":"1d","modes":["dark"]}`)
	write(opts.Bundled, "icons/material/icon.json", `{"version":"1i"}`)
	write(opts.Destination, "themes/custom/theme.json", `{"version":"owner","modes":["light","dark"],"displayName":{"zh_CN":"自定义"}}`)
	write(opts.Destination, "themes/custom/theme.css", "body{}")
	write(opts.Destination, "themes/custom/theme.js", "// custom")
	write(opts.Destination, "icons/own/icon.json", `{"version":"owner-icon"}`)
	a := conf.NewAppearance()
	a.ThemeLight = "custom"
	a.Icon = "own"
	a.Lang = "zh_CN"
	return opts, a
}

func TestAppearanceFailedWatchSetupAndRetryableClose(t *testing.T) {
	opts, a := fixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var enter, leave sync.Once
	unblock := func() { leave.Do(func() { close(release) }) }
	defer unblock()
	opts.Change = func(ctx context.Context, e Event) error {
		enter.Do(func() { close(entered) })
		<-release
		return ctx.Err()
	}
	opts.Report = func(error) {}
	o, e := New(opts)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		unblock()
		if e := o.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	if e = o.activate(context.Background(), a, []string{filepath.Join(opts.Themes, "custom"), filepath.Join(opts.Themes, "absent")}); e == nil || o.watch != nil {
		t.Fatal("failed watcher escaped publication", e)
	}
	if e = o.Apply(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(opts.Themes, "custom/theme.css"), []byte("body{color:red}"), 0600)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("callback not entered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e = o.Stop(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	unblock()
	if e = o.Close(context.Background()); e != nil {
		t.Fatal("failed close was not retryable", e)
	}
}

func TestAppearanceRepeatedWatcherReplacementDoesNotLeakDescriptors(t *testing.T) {
	fdDir := "/dev/fd"
	if runtime.GOOS == "linux" {
		fdDir = "/proc/self/fd"
	} else if runtime.GOOS != "darwin" {
		t.Skip("descriptor inventory unavailable")
	}
	opts, a := fixture(t)
	opts.Change = func(context.Context, Event) error { return nil }
	opts.Report = func(error) {}
	o, e := New(opts)
	if e != nil {
		t.Fatal(e)
	}
	defer o.Close(context.Background())
	if e = o.Apply(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	if e = o.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	// Read names only: Darwin /dev/fd entries can disappear before ReadDir's
	// fallback lstat, even when no watcher is leaking.
	readDescriptors := func() ([]string, error) {
		f, err := os.Open(fdDir)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.Readdirnames(-1)
	}
	before, e := readDescriptors()
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 20; i++ {
		if e = o.Apply(context.Background(), a); e != nil {
			t.Fatal(e)
		}
		if e = o.Stop(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	after, e := readDescriptors()
	if e != nil {
		t.Fatal(e)
	}
	if len(after) > len(before)+4 {
		t.Fatalf("watch replacement leaked descriptors: %d -> %d", len(before), len(after))
	}
}

func TestAppearanceReadOnlyMergedCatalogAndExplicitInstall(t *testing.T) {
	opts, a := fixture(t)
	o, e := New(opts)
	if e != nil {
		t.Fatal(e)
	}
	defer o.Close(context.Background())
	out, e := o.Prepare(context.Background(), a)
	if e != nil || out.ThemeVer != "owner" || !out.ThemeJS || out.IconVer != "owner-icon" {
		t.Fatal(out, e)
	}
	if _, e = os.Stat(filepath.Join(opts.Destination, "themes/daylight")); !os.IsNotExist(e) {
		t.Fatal("read installed resources", e)
	}
	opts.Change = func(context.Context, Event) error { return nil }
	opts.Report = func(error) {}
	o.options = opts
	if e = o.Apply(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(opts.Destination, "themes/daylight/theme.json")); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(opts.Destination, "themes/custom/theme.js"))
	if e != nil || string(raw) != "// custom" {
		t.Fatal("custom resource lost", e)
	}
}

func TestAppearanceOwnedWatcherSelectionStopAndRestart(t *testing.T) {
	opts, a := fixture(t)
	events := make(chan Event, 10)
	var reports atomic.Int32
	opts.Change = func(ctx context.Context, e Event) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		events <- e
		return nil
	}
	opts.Report = func(error) { reports.Add(1) }
	o, e := New(opts)
	if e != nil {
		t.Fatal(e)
	}
	defer o.Close(context.Background())
	if e = o.Apply(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(opts.Themes, "custom/theme.css")
	if e = os.WriteFile(file, []byte("body{color:red}"), 0600); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-events:
		if e.Theme != "custom" || !strings.HasPrefix(e.URL, "/appearance/themes/custom/theme.css?") {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing original CSS event")
	}
	if e = o.Stop(context.Background()); e != nil {
		t.Fatal(e)
	}
	for len(events) > 0 {
		<-events
	}
	os.WriteFile(file, []byte("body{color:blue}"), 0600)
	select {
	case e := <-events:
		t.Fatal("callback after stop", e)
	case <-time.After(80 * time.Millisecond):
	}
	a.ThemeLight = "daylight"
	if e = o.Apply(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(file, []byte("body{color:green}"), 0600)
	select {
	case e := <-events:
		t.Fatal("unselected theme emitted", e)
	case <-time.After(80 * time.Millisecond):
	}
	if e = o.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = o.Prepare(context.Background(), a); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	if reports.Load() != 0 {
		t.Fatal("watcher errors", reports.Load())
	}
}

func TestAppearanceCancellationPathsAndBadMetadata(t *testing.T) {
	opts, a := fixture(t)
	if _, e := New(Options{}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	overlapping := opts
	overlapping.Destination = opts.Bundled
	if _, e := New(overlapping); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(opts.Themes, "bad"), 0700)
	os.WriteFile(filepath.Join(opts.Themes, "bad/theme.json"), []byte("null"), 0600)
	o, e := New(opts)
	if e != nil {
		t.Fatal(e)
	}
	defer o.Close(context.Background())
	out, e := o.Prepare(context.Background(), a)
	if e != nil || len(out.LightThemes) != 2 {
		t.Fatal(out, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = o.Prepare(ctx, a); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = o.Prepare(context.Background(), a); e != nil {
		t.Fatal("cancelled gate leaked", e)
	}
	outside := t.TempDir()
	if e = os.Symlink(outside, filepath.Join(opts.Themes, "linked")); e != nil {
		t.Fatal(e)
	}
	if _, e = o.Prepare(context.Background(), a); e == nil {
		t.Fatal("unapproved linked root accepted")
	}
}
