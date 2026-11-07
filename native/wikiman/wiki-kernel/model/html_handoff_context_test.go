//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/indexrepair"
)

func htmlSessionCleanup(t *testing.T, m *markdownExportContext) {
	t.Helper()
	owner := m.read.binding.scope.commit.r.owner
	t.Cleanup(func() { owner.CloseHTMLExports(context.Background()) })
}
func htmlDownloadPath(t *testing.T, m *markdownExportContext, uri string) string {
	t.Helper()
	name, err := url.PathUnescape(strings.TrimPrefix(uri, "/export/"))
	if err != nil || strings.ContainsAny(name, "/\\") {
		t.Fatal("bad ZIP URL", uri, err)
	}
	return filepath.Join(m.read.temp, "export", name)
}
func htmlFreshRequest(t *testing.T) *markdownExportContext {
	t.Helper()
	e, err := captureArchiveExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return &markdownExportContext{read: e}
}
func TestHTMLHandoffTwoRequestsOriginalLayoutAndUniqueZIP(t *testing.T) {
	for _, mode := range []string{"html", "markdown", "pdf"} {
		t.Run(mode, func(t *testing.T) {
			m, id, before := fileContextFixture(t)
			htmlSessionCleanup(t, m)
			result, err := m.prepareHTMLFolder(id, htmlExportFileOptions{Markdown: mode == "markdown", PDF: mode == "pdf"})
			if err != nil || result.Content == "" || result.Name == "" {
				t.Fatal(result, err)
			}
			assertNoFileWork(t, m)
			folder := filepath.Join(m.read.temp, "export", result.Folder)
			if _, err = os.Stat(filepath.Join(folder, "assets/same.pdf")); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(folder, "index.html")); !os.IsNotExist(err) {
				t.Fatal("backend wrapped HTML early")
			}
			html := "<!doctype html><html><body>client wrapper " + result.Content + "</body></html>"
			next := htmlFreshRequest(t)
			uri, err := next.completeHTMLFolder(result.Folder, html, "同名 Export")
			if err != nil {
				t.Fatal(err)
			}
			zipFile := htmlDownloadPath(t, next, uri)
			got := readOutputZip(t, zipFile)
			if string(got["index.html"]) != html || string(got["assets/same.pdf"]) != "global PDF" {
				t.Fatal("ZIP content/layout")
			}
			if mode != "pdf" && string(got["stage/protyle/runtime.js"]) != "runtime" {
				t.Fatal("missing runtime")
			}
			if _, err = os.Stat(folder); !os.IsNotExist(err) {
				t.Fatal("handoff not removed", err)
			}
			if retry, e := htmlFreshRequest(t).completeHTMLFolder(result.Folder, "wrong", "same"); e == nil || retry != "" {
				t.Fatal("replay accepted")
			}
			again, err := m.prepareHTMLFolder(id, htmlExportFileOptions{Markdown: mode == "markdown", PDF: mode == "pdf"})
			if err != nil || again.Folder == result.Folder {
				t.Fatal(err)
			}
			second, err := htmlFreshRequest(t).completeHTMLFolder(again.Folder, html, "同名 Export")
			if err != nil || second == uri {
				t.Fatal("ZIP overwrite", err)
			}
			if string(readOutputZip(t, zipFile)["index.html"]) != html {
				t.Fatal("old ZIP changed")
			}
			assertNoFileWork(t, m)
			assertFileSourceUnchanged(t, before)
		})
	}
}
func TestHTMLDirectoryCheckedEnumerationBeforeCopy(t *testing.T) {
	primary := errors.New("html directory enumeration incomplete")
	for _, kind := range []string{"incomplete", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			m, _, _ := fileContextFixture(t)
			sourcePath := t.TempDir()
			if err := os.MkdirAll(filepath.Join(sourcePath, "assets"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(sourcePath, "z"), 0755); err != nil {
				t.Fatal(err)
			}
			early := filepath.Join(sourcePath, "assets/early.txt")
			if err := os.WriteFile(early, []byte("early"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sourcePath, "z/late.txt"), []byte("late"), 0644); err != nil {
				t.Fatal(err)
			}
			source, err := os.OpenRoot(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			targetPath := t.TempDir()
			target, err := os.OpenRoot(targetPath)
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			copier := markdownExportResources{
				m:      m,
				source: indexrepair.Source{Data: source},
				target: indexrepair.Source{Data: target, Authorize: func(ctx context.Context) error { return ctx.Err() }},
			}
			op := htmlDirectoryIO{readDir: func(ctx context.Context, input indexrepair.Source, name string) ([]fs.DirEntry, error) {
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
			err = copyHTMLDirectoryWithIO(&copier, op)
			if err == nil {
				t.Fatal("failed source enumeration admitted copy")
			}
			if kind == "incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete enumeration error lost or admitted as missing", err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("replaced source accepted", err)
			}
			entries, readErr := os.ReadDir(targetPath)
			if readErr != nil || len(entries) != 0 {
				t.Fatal("target changed before complete enumeration", entries, readErr)
			}
		})
	}
}
func TestHTMLHandoffRejectsUnregisteredAndReplacedDirectories(t *testing.T) {
	for _, kind := range []string{"traversal", "absolute", "foreign", "replaced", "symlink", "internal-link", "index-link", "export-root"} {
		t.Run(kind, func(t *testing.T) {
			m, id, before := fileContextFixture(t)
			htmlSessionCleanup(t, m)
			result, err := m.prepareHTMLFolder(id, htmlExportFileOptions{})
			if err != nil {
				t.Fatal(err)
			}
			folder := filepath.Join(m.read.temp, "export", result.Folder)
			foreign := t.TempDir()
			os.WriteFile(filepath.Join(foreign, "keep"), []byte("foreign"), 0644)
			argument := result.Folder
			switch kind {
			case "traversal":
				argument = "../foreign"
			case "absolute":
				argument = foreign
			case "foreign":
				argument = "other-folder"
				os.Mkdir(filepath.Join(m.read.temp, "export", argument), 0755)
			case "replaced", "symlink":
				os.Rename(folder, folder+"-old")
				if kind == "symlink" {
					os.Symlink(foreign, folder)
				} else {
					os.Mkdir(folder, 0755)
					os.WriteFile(filepath.Join(folder, "keep"), []byte("replacement"), 0644)
				}
			case "internal-link":
				os.Symlink(foreign, filepath.Join(folder, "linked"))
			case "index-link":
				os.Symlink(filepath.Join(foreign, "keep"), filepath.Join(folder, "index.html"))
			case "export-root":
				os.Rename(filepath.Join(m.read.temp, "export"), filepath.Join(m.read.temp, "export-old"))
				os.Mkdir(filepath.Join(m.read.temp, "export"), 0755)
			}
			uri, err := htmlFreshRequest(t).completeHTMLFolder(argument, "must not publish", "test")
			if err == nil || uri != "" {
				t.Fatal("unsafe callback accepted", kind, err)
			}
			value, _ := os.ReadFile(filepath.Join(foreign, "keep"))
			if string(value) != "foreign" {
				t.Fatal("foreign file touched")
			}
			if kind == "replaced" {
				value, _ = os.ReadFile(filepath.Join(folder, "keep"))
				if string(value) != "replacement" {
					t.Fatal("replacement removed")
				}
			}
			assertNoFileWork(t, m)
			assertFileSourceUnchanged(t, before)
		})
	}
}
func TestHTMLHandoffExpiryLimitAndClosedOwner(t *testing.T) {
	m, id, _ := fileContextFixture(t)
	htmlSessionCleanup(t, m)
	owner := m.read.binding.scope.commit.r.owner
	result, err := m.prepareHTMLFolder(id, htmlExportFileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owner.htmlExportMu.Lock()
	h := owner.htmlExports[result.Folder]
	h.created = time.Now().Add(-htmlHandoffTTL - time.Second)
	owner.htmlExports[result.Folder] = h
	owner.htmlExportMu.Unlock()
	if uri, e := htmlFreshRequest(t).completeHTMLFolder(result.Folder, "expired", "test"); e == nil || uri != "" {
		t.Fatal("expired accepted")
	}
	if _, e := os.Stat(filepath.Join(m.read.temp, "export", result.Folder)); !os.IsNotExist(e) {
		t.Fatal("expired folder kept")
	}
	first, e := m.prepareHTMLFolder(id, htmlExportFileOptions{})
	if e != nil {
		t.Fatal(e)
	}
	owner.htmlExportMu.Lock()
	h = owner.htmlExports[first.Folder]
	h.created = time.Now().Add(-htmlHandoffTTL - time.Second)
	owner.htmlExports[first.Folder] = h
	owner.htmlExportMu.Unlock()
	next, e := m.prepareHTMLFolder(id, htmlExportFileOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(m.read.temp, "export", first.Folder)); !os.IsNotExist(e) {
		t.Fatal("expired record not pruned")
	}
	owner.htmlExportMu.Lock()
	saved := owner.htmlExports
	owner.htmlExports = map[string]htmlExportHandoff{}
	for i := 0; i < htmlHandoffLimit; i++ {
		owner.htmlExports[fmt.Sprint(i)] = htmlExportHandoff{created: time.Now()}
	}
	owner.htmlExportMu.Unlock()
	rejected, e := m.prepareHTMLFolder(id, htmlExportFileOptions{})
	if e == nil || rejected.Folder != "" {
		t.Fatal("limit ignored")
	}
	owner.htmlExportMu.Lock()
	owner.htmlExports = saved
	owner.htmlExportMu.Unlock()
	if err = owner.CloseHTMLExports(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, e = os.Stat(filepath.Join(m.read.temp, "export", next.Folder)); !os.IsNotExist(e) {
		t.Fatal("close kept folder")
	}
	if r, e := m.prepareHTMLFolder(id, htmlExportFileOptions{}); e == nil || r.Folder != "" {
		t.Fatal("closed owner accepted")
	}
	legacyServices.Lock()
	_, registered := legacyServices.closers[htmlCloserName(owner)]
	legacyServices.Unlock()
	if registered {
		t.Fatal("closed owner retained by service registry")
	}
}
func TestHTMLHandoffConcurrentCallbacksAreSingleUse(t *testing.T) {
	m, id, _ := fileContextFixture(t)
	htmlSessionCleanup(t, m)
	r, err := m.prepareHTMLFolder(id, htmlExportFileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, b := htmlFreshRequest(t), htmlFreshRequest(t)
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make([]error, 2)
	uris := make([]string, 2)
	for i, request := range []*markdownExportContext{a, b} {
		go func(i int, request *markdownExportContext) {
			defer wg.Done()
			uris[i], errs[i] = request.completeHTMLFolder(r.Folder, "client wrapper", "duplicate")
		}(i, request)
	}
	wg.Wait()
	passed := 0
	for i, e := range errs {
		if e == nil && uris[i] != "" {
			passed++
		}
	}
	if passed != 1 {
		t.Fatal("callbacks consumed more than once", uris, errs)
	}
}
func TestHTMLHandoffCancelAndRepoCloseLifecycle(t *testing.T) {
	m, id, before := fileContextFixture(t)
	htmlSessionCleanup(t, m)
	r, err := m.prepareHTMLFolder(id, htmlExportFileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if uri, e := ExportBrowserHTMLContext(ctx, r.Folder, "client", "test"); e == nil || uri != "" {
		t.Fatal("canceled request accepted")
	}
	owner := m.read.binding.scope.commit.r.owner
	if err = owner.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, e := os.Stat(filepath.Join(m.read.temp, "export", r.Folder)); !os.IsNotExist(e) {
		t.Fatal("repo close kept handoff")
	}
	assertFileSourceUnchanged(t, before)
	if r, e := ExportHTMLFolderContext(ctx, id, false, false, false, false); e == nil || r.Folder != "" {
		t.Fatal("canceled preparation accepted")
	}
}

func TestHTMLHandoffOwnerIsolation(t *testing.T) {
	m, id, _ := fileContextFixture(t)
	htmlSessionCleanup(t, m)
	r, err := m.prepareHTMLFolder(id, htmlExportFileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owner := m.read.binding.scope.commit.r.owner
	owner.htmlExportMu.Lock()
	sessions := owner.htmlExports
	owner.htmlExports = nil
	owner.htmlExportMu.Unlock()
	if uri, e := htmlFreshRequest(t).completeHTMLFolder(r.Folder, "foreign", "test"); e == nil || uri != "" {
		t.Fatal("filesystem folder granted authority")
	}
	owner.htmlExportMu.Lock()
	owner.htmlExports = sessions
	owner.htmlExportMu.Unlock()
	if uri, e := htmlFreshRequest(t).completeHTMLFolder(r.Folder, "owned", "test"); e != nil || uri == "" {
		t.Fatal(e)
	}
}
