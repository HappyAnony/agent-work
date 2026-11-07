//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func markdownZIPFixture(t *testing.T) (*MarkdownZIPImportContext, string, string, []byte) {
	t.Helper()
	_, file, before := archiveEntryFixture(t)
	Conf.m.Lock()
	Conf.Export = conf.NewExport()
	Conf.m.Unlock()
	d, err := CaptureMarkdownZIPImport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return d, filepath.Base(filepath.Dir(file)), file, before
}
func markdownZIPBytes(t *testing.T, members ...archiveMember) []byte {
	t.Helper()
	raw, err := os.ReadFile(makeArchive(t, members...))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func assertMarkdownZIPClean(t *testing.T, d *MarkdownZIPImportContext) {
	t.Helper()
	e := d.preparation.archive
	e.scratchMu.Lock()
	n := len(e.scratch)
	e.scratchMu.Unlock()
	if n != 0 {
		t.Fatal("scratch registry retained", n)
	}
	entries, err := os.ReadDir(e.temp)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "wiki-markdown-upload-") || strings.HasPrefix(entry.Name(), "wiki-extract-") {
			t.Fatal("owned scratch leaked", entry.Name())
		}
	}
	if d.preparation.trees != nil || d.preparation.links != nil {
		t.Fatal("preparation candidates retained")
	}
}
func TestMarkdownZIPLifecycleExclusivePathsDisplayAndCompletion(t *testing.T) {
	d, box, file, before := markdownZIPFixture(t)
	raw := markdownZIPBytes(t, archiveMember{"From.md", []byte("[[Target]]"), 0}, archiveMember{"Target.markdown", []byte("# Heading"), 0}, archiveMember{"nested/", nil, os.ModeDir | 0755})
	e := d.preparation.archive
	owner := e.binding.scope.commit.r.owner
	owner.syncRuntime.same.Store(7)
	called := false
	d.importLocal = func(gotBox, folder, to, label string) error {
		called = true
		if gotBox != box || to != "/" || label != "Notes 中文 space" || !strings.HasPrefix(filepath.Base(folder), "wiki-extract-") {
			t.Fatal("filename escaped display role", gotBox, folder, to, label)
		}
		if b, err := os.ReadFile(filepath.Join(folder, "From.md")); err != nil || string(b) != "[[Target]]" {
			t.Fatal(err)
		}
		e.scratchMu.Lock()
		defer e.scratchMu.Unlock()
		if len(e.scratch) != 2 {
			t.Fatal("upload/extract not independently owned", len(e.scratch))
		}
		for _, owned := range e.scratch {
			if info, err := owned.root.Stat("."); err != nil || info.Mode().Perm() != 0700 {
				t.Fatal("scratch permissions", err)
			}
			if strings.HasPrefix(owned.name, "wiki-markdown-upload-") {
				data, err := owned.root.ReadFile("archive.zip")
				if err != nil || !bytes.Equal(data, raw) {
					t.Fatal("upload changed", err)
				}
			}
		}
		return nil
	}
	started := time.Now()
	if err := d.ImportZIPReader(bytes.NewReader(raw), box, "/", "Notes 中文 space.zip"); err != nil {
		t.Fatal(err)
	}
	if !called || owner.syncRuntime.same.Load() != 0 {
		t.Fatal("completion missing")
	}
	owner.syncRuntime.planMu.Lock()
	planned := owner.syncRuntime.plan
	owner.syncRuntime.planMu.Unlock()
	if planned.Before(started.Add(d.preparation.archive.binding.scope.commit.interval)) {
		t.Fatal("captured sync interval lost")
	}
	if b, _ := os.ReadFile(file); !bytes.Equal(b, before) {
		t.Fatal("ZIP lifecycle changed canonical source")
	}
	assertMarkdownZIPClean(t, d)
	reads := 0
	if err := d.ImportZIPReader(markdownZIPReadFunc(func([]byte) (int, error) { reads++; return 0, io.EOF }), box, "/", "again.zip"); err == nil || reads != 0 {
		t.Fatal("request importer reused", err, reads)
	}
}
func TestMarkdownZIPRejectsBadMembersCRCAndInvalidZIPBeforeImport(t *testing.T) {
	for _, kind := range []string{"traversal", "absolute", "backslash", "symlink", "pipe", "duplicate", "prefix-conflict", "crc", "invalid-zip"} {
		t.Run(kind, func(t *testing.T) {
			d, box, file, before := markdownZIPFixture(t)
			called := false
			d.importLocal = func(string, string, string, string) error { called = true; return nil }
			d.preparation.archive.binding.scope.commit.r.owner.syncRuntime.same.Store(7)
			members := []archiveMember{{"Good.md", []byte("good"), 0}}
			switch kind {
			case "traversal":
				members = append(members, archiveMember{"../escape.md", nil, 0})
			case "absolute":
				members = append(members, archiveMember{"/escape.md", nil, 0})
			case "backslash":
				members = append(members, archiveMember{"dir\\escape.md", nil, 0})
			case "symlink":
				members = append(members, archiveMember{"alias", []byte("outside"), os.ModeSymlink | 0777})
			case "pipe":
				members = append(members, archiveMember{"pipe", nil, os.ModeNamedPipe | 0600})
			case "duplicate":
				members = append(members, members[0])
			case "prefix-conflict":
				members = append(members, archiveMember{"parent", []byte("file"), 0}, archiveMember{"parent/child.md", nil, 0})
			case "crc":
				members[0].data = []byte("unique-markdown-crc-payload")
			}
			raw := markdownZIPBytes(t, members...)
			if kind == "crc" {
				i := bytes.Index(raw, members[0].data)
				if i < 0 {
					t.Fatal("fixture")
				}
				raw[i] ^= 1
			}
			if kind == "invalid-zip" {
				raw = []byte("not ZIP")
			}
			err := d.ImportZIPReader(bytes.NewReader(raw), box, "/", "../../unsafe.zip")
			if err == nil || called || d.preparation.archive.binding.scope.commit.r.owner.syncRuntime.same.Load() != 7 {
				t.Fatal("bad ZIP imported/completed", err, called)
			}
			if after, _ := os.ReadFile(file); !bytes.Equal(after, before) {
				t.Fatal("bad ZIP changed source")
			}
			assertMarkdownZIPClean(t, d)
		})
	}
}

type markdownZIPReadFunc func([]byte) (int, error)

func (f markdownZIPReadFunc) Read(p []byte) (int, error) { return f(p) }
func TestMarkdownZIPCancellationOwnerReadonlyAndReaderFailures(t *testing.T) {
	for _, kind := range []string{"canceled", "cancel-copy", "owner", "temp", "readonly", "reader", "destination", "nil-reader"} {
		t.Run(kind, func(t *testing.T) {
			d, box, _, _ := markdownZIPFixture(t)
			called := false
			d.importLocal = func(string, string, string, string) error { called = true; return nil }
			owner := d.preparation.archive.binding.scope.commit.r.owner
			owner.syncRuntime.same.Store(7)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d.preparation.ctx = ctx
			d.preparation.archive.ctx = ctx
			sentinel := errors.New("upload reader failed")
			var reader io.Reader = bytes.NewReader([]byte("ZIP"))
			switch kind {
			case "canceled":
				cancel()
			case "cancel-copy":
				reader = markdownZIPReadFunc(func([]byte) (int, error) { cancel(); return 0, io.EOF })
			case "owner":
				old := Conf
				Conf = NewAppConf()
				defer func() { Conf = old }()
			case "temp":
				old := util.TempDir
				util.TempDir = t.TempDir()
				defer func() { util.TempDir = old }()
			case "readonly":
				owner.m.Lock()
				owner.persistenceReadOnly = true
				owner.m.Unlock()
				defer func() { owner.m.Lock(); owner.persistenceReadOnly = false; owner.m.Unlock() }()
			case "reader":
				reader = markdownZIPReadFunc(func([]byte) (int, error) { return 0, sentinel })
			case "destination":
				box = "../other"
			case "nil-reader":
				reader = nil
			}
			err := d.ImportZIPReader(reader, box, "/", "notes.zip")
			if err == nil || called || owner.syncRuntime.same.Load() != 7 {
				t.Fatal(err, called)
			}
			if (kind == "canceled" || kind == "cancel-copy") && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if kind == "reader" && !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			assertMarkdownZIPClean(t, d)
		})
	}
}
func TestMarkdownZIPInputBorrowCloseFailureStopsImportAndCleans(t *testing.T) {
	d, box, _, _ := markdownZIPFixture(t)
	sentinel := errors.New("borrowed input root close")
	d.preparation.archive.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), sentinel) }
	d.preparation.archive.binding.scope.commit.r.owner.syncRuntime.same.Store(7)
	d.importLocal = func(string, string, string, string) error { t.Fatal("closed input handed to importer"); return nil }
	err := d.ImportZIPReader(bytes.NewReader(markdownZIPBytes(t, archiveMember{"A.md", []byte("A"), 0})), box, "/", "notes.zip")
	if !errors.Is(err, sentinel) || d.preparation.archive.binding.scope.commit.r.owner.syncRuntime.same.Load() != 7 {
		t.Fatal(err)
	}
	assertMarkdownZIPClean(t, d)
}
func TestMarkdownZIPImportAndCleanupFailuresJoinWithoutCompletion(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses cleanup permissions")
	}
	for _, primary := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup", true: "primary-and-cleanup"}[primary], func(t *testing.T) {
			d, box, _, _ := markdownZIPFixture(t)
			e := d.preparation.archive
			e.binding.scope.commit.r.owner.syncRuntime.same.Store(7)
			sentinel := errors.New("local import failed")
			d.importLocal = func(string, string, string, string) error {
				if err := os.Chmod(e.temp, 0500); err != nil {
					t.Fatal(err)
				}
				if primary {
					return sentinel
				}
				return nil
			}
			defer func() {
				os.Chmod(e.temp, 0700)
				entries, _ := os.ReadDir(e.temp)
				for _, entry := range entries {
					if strings.HasPrefix(entry.Name(), "wiki-markdown-upload-") || strings.HasPrefix(entry.Name(), "wiki-extract-") {
						os.RemoveAll(filepath.Join(e.temp, entry.Name()))
					}
				}
			}()
			err := d.ImportZIPReader(bytes.NewReader(markdownZIPBytes(t, archiveMember{"A.md", []byte("A"), 0})), box, "/", "notes.zip")
			if !errors.Is(err, os.ErrPermission) || primary && !errors.Is(err, sentinel) || e.binding.scope.commit.r.owner.syncRuntime.same.Load() != 7 {
				t.Fatal("failure lost or completed", err)
			}
			if d.preparation.trees != nil || d.preparation.links != nil {
				t.Fatal("candidate residue")
			}
		})
	}
}
func TestMarkdownZIPConcurrentSameFilenameKeepsIndependentScratch(t *testing.T) {
	first, box, _, _ := markdownZIPFixture(t)
	const count = 4
	importers := []*MarkdownZIPImportContext{first}
	for i := 1; i < count; i++ {
		d, err := CaptureMarkdownZIPImport(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		importers = append(importers, d)
	}
	raw := markdownZIPBytes(t, archiveMember{"A.md", []byte("A"), 0})
	paths := make(chan string, count)
	release := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, count)
	var once sync.Once
	releaseAll := func() { once.Do(func() { close(release) }) }
	defer func() { releaseAll(); wg.Wait() }()
	for _, d := range importers {
		d.importLocal = func(_, folder, _, label string) error {
			if label != "same-name" {
				t.Error(label)
			}
			paths <- folder
			<-release
			return nil
		}
		wg.Add(1)
		go func(d *MarkdownZIPImportContext) {
			defer wg.Done()
			results <- d.ImportZIPReader(bytes.NewReader(raw), box, "/", "same-name.zip")
		}(d)
	}
	seen := map[string]bool{}
	for i := 0; i < count; i++ {
		select {
		case p := <-paths:
			if seen[p] {
				t.Error("shared extraction", p)
			}
			seen[p] = true
		case <-time.After(10 * time.Second):
			releaseAll()
			t.Fatal("import callback not reached")
		}
	}
	releaseAll()
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range importers {
		assertMarkdownZIPClean(t, d)
	}
}
func TestMarkdownZIPDisplayFilenameNeverSelectsDiskPath(t *testing.T) {
	for _, name := range []string{"../../escape.zip", "C:\\folder\\escape.zip", "Notes space 中文.zip", ".zip"} {
		t.Run(name, func(t *testing.T) {
			d, box, _, _ := markdownZIPFixture(t)
			d.importLocal = func(_, folder, _, label string) error {
				if strings.ContainsAny(label, "/\\") || label == "" || folder == name || strings.Contains(filepath.Base(folder), "escape") {
					t.Fatal("filename became a path", folder, label)
				}
				if name == "Notes space 中文.zip" && label != "Notes space 中文" {
					t.Fatal(label)
				}
				return nil
			}
			if err := d.ImportZIPReader(bytes.NewReader(markdownZIPBytes(t, archiveMember{"A.md", []byte("A"), 0})), box, "/", name); err != nil {
				t.Fatal(err)
			}
			assertMarkdownZIPClean(t, d)
		})
	}
}
func TestMarkdownZIPActualLocalImportKeepsDisplayAndPreparation(t *testing.T) {
	d, box, _, _ := markdownZIPFixture(t)
	raw := markdownZIPBytes(t, archiveMember{"From.md", []byte("[[Target]]"), 0}, archiveMember{"Target.markdown", []byte("Target"), 0})
	if err := d.ImportZIPReader(bytes.NewReader(raw), box, "/", "Original label.zip"); err != nil {
		t.Fatal(err)
	}
	titles := map[string]bool{}
	if err := filepath.WalkDir(filepath.Join(d.preparation.archive.binding.scope.commit.r.data, box), func(p string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(p, ".docj") {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if bytes.Contains(b, []byte("Original label")) {
				titles["label"] = true
			}
			if bytes.Contains(b, []byte("wiki-extract-")) {
				t.Error("internal scratch name leaked into document")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !titles["label"] {
		t.Fatal("original ZIP display root lost")
	}
	assertMarkdownZIPClean(t, d)
}
