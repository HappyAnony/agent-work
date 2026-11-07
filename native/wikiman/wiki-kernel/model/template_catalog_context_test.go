package model

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"wiki-kernel/conf"
)

func TestTemplateCatalogActualSearchRemoveAndReadOnly(t *testing.T) {
	c, _, paths := repoMetadataFixture(t)
	c.Search = conf.NewSearch()
	base := filepath.Join(paths.Data, "templates")
	for _, name := range []string{"Daily.md", "README.md", ".hidden.md", "group/Nested.md", "group/readme-any.md", "group/.hidden/Secret.md"} {
		p := filepath.Join(base, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(base, "group", "Directory.md"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := SearchTemplateContext(context.Background(), "")
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	legacy := SearchTemplate("")
	if !reflect.DeepEqual(got, legacy) {
		t.Fatalf("legacy difference: %v %v", got, legacy)
	}
	got, err = SearchTemplateContext(context.Background(), "nested")
	if err != nil || len(got) != 1 || !strings.Contains(got[0].Content, "<mark>") {
		t.Fatalf("%v %v", got, err)
	}
	p := filepath.Join(base, "Daily.md")
	c.persistenceReadOnly = true
	if err = RemoveTemplateContext(context.Background(), p); !errors.Is(err, ErrSettingReadOnly) {
		t.Fatal(err)
	}
	c.persistenceReadOnly = false
	if err = RemoveTemplateContext(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err = RemoveTemplateContext(context.Background(), p); err != nil {
		t.Fatal("missing delete not idempotent", err)
	}
	if err = RemoveTemplateContext(context.Background(), filepath.Join(base, "group")); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateCatalogRejectsOutsidePathsAndReplacement(t *testing.T) {
	c, _, paths := repoMetadataFixture(t)
	c.Search = conf.NewSearch()
	base := filepath.Join(paths.Data, "templates")
	if err := os.Mkdir(base, 0700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	file := filepath.Join(outside, "keep.md")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "outside")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{file, base, filepath.Join(base, "outside", "keep.md")} {
		if err := RemoveTemplateContext(context.Background(), p); err == nil {
			t.Fatalf("outside path accepted: %s", p)
		}
	}
	if raw, err := os.ReadFile(file); err != nil || string(raw) != "keep" {
		t.Fatal(string(raw), err)
	}
	o, err := captureTemplateCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	Conf = NewAppConf()
	defer func() { Conf = c }()
	called := false
	if err = o.run(context.Background(), func(context.Context) error { called = true; return nil }); err == nil || called {
		t.Fatal(err, called)
	}
}

func TestTemplateCatalogCancellationAndActualJoin(t *testing.T) {
	c, _, _ := repoMetadataFixture(t)
	c.Search = conf.NewSearch()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SearchTemplateContext(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := RemoveTemplateContext(ctx, "/unused"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	o, err := captureTemplateCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entered, cancelled, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan error, 1)
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	go func() {
		done <- o.run(context.Background(), func(op context.Context) error {
			close(entered)
			<-op.Done()
			close(cancelled)
			<-release
			return op.Err()
		})
	}()
	<-entered
	deadline, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if err = c.CloseCloudRepo(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	<-cancelled
	close(release)
	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = c.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = o.run(context.Background(), func(context.Context) error { t.Fatal("late operation"); return nil }); err == nil {
		t.Fatal("closed owner accepted operation")
	}
}

func TestTemplateCatalogEnumerationErrorsDiscardPartialResult(t *testing.T) {
	fake := fstest.MapFS{"Good.md": &fstest.MapFile{}, "group/Other.md": &fstest.MapFile{}}
	read := func(string) ([]os.DirEntry, error) { return fs.ReadDir(fake, ".") }
	failure := errors.New("synthetic enumeration failure")
	got, err := searchTemplateCatalog(context.Background(), "base", "", false, read, func(string, fs.WalkDirFunc) error { return failure })
	if !errors.Is(err, failure) || got != nil {
		t.Fatal(got, err)
	}
	got, err = searchTemplateCatalog(context.Background(), "base", "", false, func(string) ([]os.DirEntry, error) { return nil, failure }, nil)
	if !errors.Is(err, failure) || got != nil {
		t.Fatal(got, err)
	}
}
