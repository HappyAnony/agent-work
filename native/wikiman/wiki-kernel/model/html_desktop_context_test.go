//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/internal/indexrepair"
)

func TestHTMLDesktopOriginalResourcesMergeAndResponse(t *testing.T) {
	for _, mode := range []string{"html", "markdown", "pdf"} {
		t.Run(mode, func(t *testing.T) {
			m, id, before := fileContextFixture(t)
			dest := filepath.Join(t.TempDir(), "新目录 with spaces", "nested")
			opt := htmlExportFileOptions{Markdown: mode == "markdown", PDF: mode == "pdf"}
			r, e := m.exportHTMLDesktop(id, dest, opt)
			if e != nil || r.Name == "" || r.Content == "" || r.Folder != "" {
				t.Fatal(r, e)
			}
			got, e := os.ReadFile(filepath.Join(dest, "assets/same.pdf"))
			if e != nil || string(got) != "global PDF" {
				t.Fatal(e)
			}
			if _, e = os.Stat(filepath.Join(dest, "index.html")); !os.IsNotExist(e) {
				t.Fatal("backend wrote frontend wrapper")
			}
			if mode != "pdf" {
				if _, e = os.Stat(filepath.Join(dest, "stage/protyle/runtime.js")); e != nil {
					t.Fatal(e)
				}
			}
			os.WriteFile(filepath.Join(dest, "keep"), []byte("keep"), 0644)
			os.WriteFile(filepath.Join(dest, "index.html"), []byte("index"), 0644)
			os.WriteFile(filepath.Join(dest, "assets/same.pdf"), []byte("old"), 0644)
			again, e := m.exportHTMLDesktop(id, dest, opt)
			if e != nil || again.Name != r.Name {
				t.Fatal(e)
			}
			for file, want := range map[string]string{"keep": "keep", "index.html": "index", "assets/same.pdf": "global PDF"} {
				got, _ := os.ReadFile(filepath.Join(dest, file))
				if string(got) != want {
					t.Fatal(file)
				}
			}
			assertNoFileWork(t, m)
			assertFileSourceUnchanged(t, before)
		})
	}
}
func TestHTMLDesktopRejectsProtectedAndInvalidDestinations(t *testing.T) {
	for _, kind := range []string{"relative", "traversal", "file", "link", "data", "workspace-parent", "work", "source-alias", "case-alias"} {
		t.Run(kind, func(t *testing.T) {
			m, id, before := fileContextFixture(t)
			out := t.TempDir()
			dest := filepath.Join(out, "new")
			switch kind {
			case "relative":
				dest = "relative/output"
			case "traversal":
				dest = out + "/../escape"
			case "file":
				dest = filepath.Join(out, "file")
				os.WriteFile(dest, []byte("keep"), 0644)
			case "link":
				dest = filepath.Join(out, "alias")
				os.Symlink(out, dest)
			case "data":
				dest = m.read.binding.scope.commit.r.data
			case "case-alias":
				dest = strings.ToUpper(m.read.binding.scope.commit.r.data)
			case "workspace-parent":
				dest = filepath.Dir(m.read.binding.scope.commit.r.workspace)
			case "work":
				dest = m.htmlResources.work.path
			case "source-alias":
				alias := filepath.Join(out, "alias")
				os.Symlink(m.read.binding.scope.commit.r.data, alias)
				dest = filepath.Join(alias, "export")
			}
			r, e := m.exportHTMLDesktop(id, dest, htmlExportFileOptions{})
			if e == nil || r.Name != "" || r.Content != "" {
				t.Fatal("unsafe destination accepted", kind, e)
			}
			assertNoFileWork(t, m)
			assertFileSourceUnchanged(t, before)
		})
	}
}
func TestHTMLDesktopPreflightRejectsBeforeCopies(t *testing.T) {
	for _, kind := range []string{"index-link", "index-dir", "asset-link", "dir-collision", "file-collision"} {
		t.Run(kind, func(t *testing.T) {
			m, id, before := fileContextFixture(t)
			dest := t.TempDir()
			foreign := filepath.Join(t.TempDir(), "keep")
			os.WriteFile(foreign, []byte("foreign"), 0644)
			os.Mkdir(filepath.Join(dest, "assets"), 0755)
			os.WriteFile(filepath.Join(dest, "assets/same.pdf"), []byte("old"), 0644)
			switch kind {
			case "index-link":
				os.Symlink(foreign, filepath.Join(dest, "index.html"))
			case "index-dir":
				os.Mkdir(filepath.Join(dest, "index.html"), 0755)
			case "asset-link":
				os.Remove(filepath.Join(dest, "assets/same.pdf"))
				os.Symlink(foreign, filepath.Join(dest, "assets/same.pdf"))
			case "dir-collision":
				os.WriteFile(filepath.Join(dest, "stage"), []byte("file"), 0644)
			case "file-collision":
				os.MkdirAll(filepath.Join(dest, "stage/protyle/runtime.js"), 0755)
			}
			r, e := m.exportHTMLDesktop(id, dest, htmlExportFileOptions{})
			if e == nil || r.Content != "" {
				t.Fatal("conflict hidden")
			}
			got, _ := os.ReadFile(foreign)
			if string(got) != "foreign" {
				t.Fatal("link changed")
			}
			if kind != "asset-link" {
				got, _ = os.ReadFile(filepath.Join(dest, "assets/same.pdf"))
				if string(got) != "old" {
					t.Fatal("copy before full preflight")
				}
			}
			assertNoFileWork(t, m)
			assertFileSourceUnchanged(t, before)
		})
	}
}
func TestHTMLDesktopCheckedPreflightEnumerationBeforeCopy(t *testing.T) {
	primary := errors.New("html desktop preflight enumeration incomplete")
	fixture, _, _ := fileContextFixture(t)
	if _, err := fixture.read.resourceManifest(indexrepair.Source{}, ".", nil, nil, true); !errors.Is(err, indexrepair.ErrSourcePath) {
		t.Fatal("nil independent root accepted", err)
	}
	for _, kind := range []string{"incomplete", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			m, _, _ := fileContextFixture(t)
			dest := t.TempDir()
			if err := os.Mkdir(filepath.Join(dest, "assets"), 0755); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dest, "assets/early.txt")
			if err := os.WriteFile(target, []byte("old"), 0644); err != nil {
				t.Fatal(err)
			}
			destination, err := m.captureHTMLDesktopDestination(dest)
			if err != nil {
				t.Fatal(err)
			}
			if err = destination.open(); err != nil {
				t.Fatal(err)
			}
			defer destination.root.Close()

			sourcePath := t.TempDir()
			if err = os.MkdirAll(filepath.Join(sourcePath, "assets"), 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.MkdirAll(filepath.Join(sourcePath, "z"), 0755); err != nil {
				t.Fatal(err)
			}
			early := filepath.Join(sourcePath, "assets/early.txt")
			if err = os.WriteFile(early, []byte("new"), 0644); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(sourcePath, "z/late.txt"), []byte("late"), 0644); err != nil {
				t.Fatal(err)
			}
			source, err := os.OpenRoot(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()

			op := htmlDesktopIO{preflightReadDir: func(ctx context.Context, input indexrepair.Source, name string) ([]fs.DirEntry, error) {
				if name != "z" {
					return input.ReadDir(ctx, name)
				}
				if kind == "incomplete" {
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
				}
				entries, readErr := input.ReadDir(ctx, name)
				if readErr != nil {
					return nil, readErr
				}
				if err = os.Rename(early, early+".old"); err != nil {
					return nil, err
				}
				if err = os.WriteFile(early, []byte("replacement"), 0644); err != nil {
					return nil, err
				}
				return entries, nil
			}}
			err = destination.preflightWithIO(source, op)
			if err == nil {
				t.Fatal("failed source enumeration admitted desktop copy")
			}
			if kind == "incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete source enumeration error lost or admitted as missing", err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("replaced source accepted", err)
			}
			got, readErr := os.ReadFile(target)
			if readErr != nil || string(got) != "old" {
				t.Fatal("destination changed before complete preflight", string(got), readErr)
			}
		})
	}
}
func TestHTMLDesktopCapturedDestinationIdentity(t *testing.T) {
	for _, kind := range []string{"root", "alias", "child-link"} {
		t.Run(kind, func(t *testing.T) {
			m, _, _ := fileContextFixture(t)
			parent := t.TempDir()
			dest := filepath.Join(parent, "output")
			os.Mkdir(dest, 0755)
			if kind == "alias" {
				alias := filepath.Join(t.TempDir(), "alias")
				os.Symlink(parent, alias)
				dest = filepath.Join(alias, "output")
			}
			if kind == "child-link" {
				dest = filepath.Join(parent, "missing", "child")
			}
			d, e := m.captureHTMLDesktopDestination(dest)
			if e != nil {
				t.Fatal(e)
			}
			if kind == "child-link" {
				os.Symlink(t.TempDir(), filepath.Join(parent, "missing"))
				if e = d.open(); e == nil {
					d.root.Close()
					t.Fatal("child link accepted")
				}
				return
			}
			if e = d.open(); e != nil {
				t.Fatal(e)
			}
			defer d.root.Close()
			if kind == "root" {
				os.Rename(dest, dest+"-old")
				os.Mkdir(dest, 0755)
			} else {
				alias := filepath.Dir(dest)
				os.Remove(alias)
				os.Symlink(t.TempDir(), alias)
			}
			if e = d.check(); e == nil {
				t.Fatal("identity change accepted")
			}
		})
	}
}
func TestHTMLDesktopFailureBeforeDestinationCreation(t *testing.T) {
	for _, kind := range []string{"missing", "resource", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			m, id, before := fileContextFixture(t)
			dest := filepath.Join(t.TempDir(), "new")
			switch kind {
			case "missing":
				id = "20261101170000-missing"
			case "resource":
				os.RemoveAll(filepath.Join(m.htmlResources.work.path, "stage/protyle"))
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				m.read.binding.scope.ctx = ctx
			}
			r, e := m.exportHTMLDesktop(id, dest, htmlExportFileOptions{})
			if e == nil || r.Content != "" {
				t.Fatal("hidden failure")
			}
			if _, e = os.Stat(dest); !os.IsNotExist(e) {
				t.Fatal("output created before preparation", e)
			}
			assertNoFileWork(t, m)
			if kind != "resource" {
				assertFileSourceUnchanged(t, before)
			}
		})
	}
	if r, e := ExportHTMLDesktopContext(nil, "id", "/tmp/out", false, false, false, false); e == nil || r.Content != "" {
		t.Fatal("nil context")
	}
}
func TestHTMLDesktopParentAliasAndPrefixSibling(t *testing.T) {
	m, id, before := fileContextFixture(t)
	parent := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	os.Symlink(parent, alias)
	r, e := m.exportHTMLDesktop(id, filepath.Join(alias, "nested/output"), htmlExportFileOptions{})
	if e != nil || !strings.Contains(r.Content, "File heading") {
		t.Fatal(e)
	}
	got, e := os.ReadFile(filepath.Join(parent, "nested/output/assets/same.pdf"))
	if e != nil || string(got) != "global PDF" {
		t.Fatal(e)
	}
	if d, e := m.captureHTMLDesktopDestination(m.htmlResources.work.path + "-export"); e != nil || d == nil {
		t.Fatal("prefix sibling rejected", e)
	}
	assertFileSourceUnchanged(t, before)
}
