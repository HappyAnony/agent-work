// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func sourceFixture(t *testing.T) Source {
	t.Helper()
	r, e := os.OpenRoot(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	a, e := os.OpenRoot(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { a.Close() })
	return Source{Data: r, ArchiveRoot: a, Authorize: func(ctx context.Context) error { return ctx.Err() }}
}
func TestRepairSourceExplicitCapabilityRevisionAndCleanup(t *testing.T) {
	s := sourceFixture(t)
	ctx := context.Background()
	if e := s.Data.WriteFile("doc", []byte("old"), 0600); e != nil {
		t.Fatal(e)
	}
	reader := Source{Data: s.Data}
	if e := reader.Rewrite(ctx, "doc", []byte("old"), []byte("new")); !errors.Is(e, ErrSourceWrite) {
		t.Fatal(e)
	}
	if e := s.Rewrite(ctx, "doc", []byte("stale"), []byte("new")); !errors.Is(e, ErrSourceChanged) {
		t.Fatal(e)
	}
	if e := s.Rewrite(ctx, "doc", nil, []byte("new")); !errors.Is(e, ErrSourceChanged) {
		t.Fatal(e)
	}
	if e := s.Rewrite(ctx, "doc", []byte("old"), []byte("new")); e != nil {
		t.Fatal(e)
	}
	b, _ := s.Data.ReadFile("doc")
	if string(b) != "new" {
		t.Fatal(string(b))
	}
	info, _ := s.Data.Stat("doc")
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	entries, _ := s.Data.Open(".")
	defer entries.Close()
	names, _ := entries.Readdirnames(-1)
	if len(names) != 1 {
		t.Fatal(names)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e := s.Remove(canceled, "doc"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestRepairSourceRejectsEscapesAndHistorySymlinks(t *testing.T) {
	s := sourceFixture(t)
	ctx := context.Background()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "keep"), []byte("safe"), 0600)
	if e := s.Data.Symlink(outside, "link"); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"../keep", "/keep", "link/keep", "a\\b"} {
		if _, e := s.Read(ctx, name); e == nil {
			t.Fatal(name)
		}
	}
	s.Data.MkdirAll("box/.siyuan/history", 0700)
	s.Data.Symlink(outside, "box/.siyuan/history/nested")
	if e := s.RemoveLegacyHistory(ctx, "box"); !errors.Is(e, ErrSourcePath) {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(outside, "keep")); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Data.Lstat("box/.siyuan/history/nested"); e != nil {
		t.Fatal(e)
	}
}
func TestRepairSourceArchiveUniqueAndFailurePreservesSource(t *testing.T) {
	s := sourceFixture(t)
	ctx := context.Background()
	s.Data.MkdirAll("box", 0700)
	var paths []string
	for i := 0; i < 2; i++ {
		s.Data.WriteFile("box/bad.docj", []byte("payload"), 0600)
		p, e := s.Archive(ctx, "box/bad.docj", "same-second")
		if e != nil {
			t.Fatal(e)
		}
		paths = append(paths, p)
		b, e := s.ArchiveRoot.ReadFile(p)
		if e != nil || string(b) != "payload" {
			t.Fatal(e)
		}
	}
	if paths[0] == paths[1] {
		t.Fatal("archive collision")
	}
	s.Data.WriteFile("box/bad.docj", []byte("keep"), 0600)
	s.ArchiveRoot.Close()
	if _, e := s.Archive(ctx, "box/bad.docj", "stamp"); e == nil {
		t.Fatal("closed archive accepted")
	}
	b, e := s.Data.ReadFile("box/bad.docj")
	if e != nil || string(b) != "keep" {
		t.Fatal(e)
	}
}
