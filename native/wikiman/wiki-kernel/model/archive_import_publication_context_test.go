//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func archiveWrite(t *testing.T, root, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestArchivePublicationResourcesMergeAndExactDirectorySelection(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	root := t.TempDir()
	archiveWrite(t, root, "assets/existing.txt", []byte("incoming"))
	archiveWrite(t, root, "assets/nested/file", []byte("nested"))
	os.MkdirAll(filepath.Join(root, "assets/empty"), 0755)
	archiveWrite(t, root, "other/assets/second", []byte("second"))
	near := archiveWrite(t, root, "myassets/doc", []byte("keep source"))
	data := e.binding.scope.commit.r.data
	archiveWrite(t, data, "assets/existing.txt", []byte("old"))
	archiveWrite(t, data, "assets/untouched", []byte("keep"))
	if err := e.publishArchiveResources(root, "assets"); err != nil {
		t.Fatal(err)
	}
	for n, want := range map[string]string{"existing.txt": "incoming", "nested/file": "nested", "second": "second", "untouched": "keep"} {
		raw, err := os.ReadFile(filepath.Join(data, "assets", n))
		if err != nil || string(raw) != want {
			t.Fatal(n, string(raw), err)
		}
	}
	if info, err := os.Stat(filepath.Join(data, "assets/empty")); err != nil || !info.IsDir() {
		t.Fatal("empty directory lost", err)
	}
	if _, err := os.Stat(filepath.Join(root, "assets")); !os.IsNotExist(err) {
		t.Fatal("published scratch retained", err)
	}
	if _, err := os.Stat(near); err != nil {
		t.Fatal("substring directory moved", err)
	}
	if _, err := os.Stat(filepath.Join(data, "assets/doc")); !os.IsNotExist(err) {
		t.Fatal("substring directory published", err)
	}
}
func TestArchivePublicationEmojiSanitizationAndCollision(t *testing.T) {
	for _, collision := range []bool{false, true} {
		t.Run(map[bool]string{false: "nested", true: "collision"}[collision], func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			bad := "bad?.png"
			good := util.FilterUploadEmojiFileName(bad)
			if good == bad || good == "" {
				t.Fatal("invalid sanitization fixture")
			}
			archiveWrite(t, root, "emojis/"+bad, []byte("emoji"))
			if collision {
				archiveWrite(t, root, "emojis/"+good, []byte("existing"))
				if err := e.publishArchiveResources(root, "emojis"); !errors.Is(err, os.ErrExist) {
					t.Fatal(err)
				}
				raw, _ := os.ReadFile(filepath.Join(root, "emojis", good))
				if string(raw) != "existing" {
					t.Fatal("sanitization overwrote a different file")
				}
				return
			}
			folder := "folder?"
			clean := util.FilterUploadEmojiFileName(folder)
			archiveWrite(t, root, "emojis/"+folder+"/"+bad, []byte("nested"))
			if err := e.publishArchiveResources(root, "emojis"); err != nil {
				t.Fatal(err)
			}
			for _, n := range []string{good, clean + "/" + good} {
				if _, err := os.Stat(filepath.Join(util.DataDir, "emojis", n)); err != nil {
					t.Fatal(n, err)
				}
			}
		})
	}
}

func TestArchiveEmojiPerDirectoryPlanFailures(t *testing.T) {
	for _, kind := range []string{"read-incomplete", "collision", "later-rename"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			folder := t.TempDir()
			firstName, secondName := "a?.png", "b?.png"
			first := archiveWrite(t, folder, "emojis/"+firstName, []byte("first"))
			second := archiveWrite(t, folder, "emojis/"+secondName, []byte("second"))
			if kind == "collision" {
				archiveWrite(t, folder, "emojis/"+util.FilterUploadEmojiFileName(firstName), []byte("existing"))
			}
			root, err := os.OpenRoot(folder)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			op := archiveEmojiIO{}
			if kind == "read-incomplete" {
				op.readDir = func(context.Context, indexrepair.Source, string) ([]os.DirEntry, error) {
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist)
				}
			}
			if kind == "later-rename" {
				calls := 0
				op.rename = func(root *os.Root, from, to string) error {
					calls++
					if calls == 2 {
						return errors.New("later emoji rename failed")
					}
					return root.Rename(from, to)
				}
			}
			if err = e.sanitizeArchiveEmojisWithIO(root, "emojis", op); err == nil {
				t.Fatal("emoji plan failure became success")
			}
			firstClean := filepath.Join(folder, "emojis", util.FilterUploadEmojiFileName(firstName))
			if kind == "later-rename" {
				if _, err = os.Stat(firstClean); err != nil {
					t.Fatal("successful rename prefix lost", err)
				}
				if _, err = os.Stat(second); err != nil {
					t.Fatal("failed rename source lost", err)
				}
			} else {
				if _, err = os.Stat(first); err != nil {
					t.Fatal("failed plan renamed earlier entry", err)
				}
				if _, err = os.Stat(second); err != nil {
					t.Fatal("failed plan changed later entry", err)
				}
			}
		})
	}
}
func TestArchivePublicationRejectsLinksAndRetainsFailedScratch(t *testing.T) {
	for _, kind := range []string{"source-link", "root-link", "target-link", "target-parent-link", "target-directory", "permission"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			file := archiveWrite(t, root, "assets/a", []byte("incoming"))
			outside := t.TempDir()
			external := archiveWrite(t, outside, "external", []byte("external"))
			dest := filepath.Join(util.DataDir, "assets")
			os.MkdirAll(dest, 0755)
			switch kind {
			case "source-link":
				os.Remove(file)
				os.Symlink(external, file)
			case "root-link":
				original := root
				root = filepath.Join(t.TempDir(), "linked")
				os.Symlink(original, root)
			case "target-link":
				os.Symlink(external, filepath.Join(dest, "a"))
			case "target-parent-link":
				os.Remove(dest)
				os.Symlink(outside, dest)
			case "target-directory":
				os.Mkdir(filepath.Join(dest, "a"), 0755)
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses mode bits")
				}
				os.Chmod(dest, 0500)
				defer os.Chmod(dest, 0755)
			}
			if err := e.publishArchiveResources(root, "assets"); err == nil {
				t.Fatal("invalid resource publication accepted", kind)
			}
			if _, err := os.Lstat(file); err != nil {
				t.Fatal("failed source removed", err)
			}
			raw, _ := os.ReadFile(external)
			if string(raw) != "external" {
				t.Fatal("external link target modified")
			}
		})
	}
}
func TestArchivePublicationPartialMergeIsExplicit(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	root := t.TempDir()
	archiveWrite(t, root, "assets/a", []byte("first"))
	archiveWrite(t, root, "assets/z", []byte("last"))
	os.MkdirAll(filepath.Join(util.DataDir, "assets/z"), 0755)
	if err := e.publishArchiveResources(root, "assets"); err == nil {
		t.Fatal("late error hidden")
	}
	raw, _ := os.ReadFile(filepath.Join(util.DataDir, "assets/a"))
	if string(raw) != "first" {
		t.Fatal("earlier write unexpectedly rolled back")
	}
	if _, err := os.Stat(filepath.Join(root, "assets/a")); err != nil {
		t.Fatal("failed directory prematurely removed", err)
	}
}

func TestArchiveResourceDiscoveryAndRemovalAdmission(t *testing.T) {
	for _, kind := range []string{"walk-incomplete", "scan-replaced", "merge-failure", "post-merge-replaced", "remove-failure"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			candidate := filepath.Join(root, "a/assets")
			archiveWrite(t, root, "a/assets/file", []byte("resource"))
			archiveWrite(t, root, "z/later/file", []byte("later"))
			merged, removed := 0, 0
			op := archiveResourceIO{}
			switch kind {
			case "walk-incomplete":
				sentinel := errors.New("resource discovery incomplete")
				op.walk.readDir = func(ctx context.Context, source indexrepair.Source, current string) ([]os.DirEntry, error) {
					if current == "z/later" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, sentinel)
					}
					return source.ReadDir(ctx, current)
				}
				op.merge = func(string, string) error { merged++; return nil }
				op.removeAll = func(*os.Root, string) error { removed++; return nil }
			case "scan-replaced":
				replaced := false
				op.walk.readDir = func(ctx context.Context, source indexrepair.Source, current string) ([]os.DirEntry, error) {
					if current == "z/later" && !replaced {
						replaced = true
						if err := os.RemoveAll(candidate); err != nil {
							t.Fatal(err)
						}
						archiveWrite(t, root, "a/assets/replacement", []byte("replacement"))
					}
					return source.ReadDir(ctx, current)
				}
				op.merge = func(string, string) error { merged++; return nil }
				op.removeAll = func(*os.Root, string) error { removed++; return nil }
			case "merge-failure":
				op.merge = func(string, string) error { merged++; return errors.New("resource merge failed") }
				op.removeAll = func(*os.Root, string) error { removed++; return nil }
			case "post-merge-replaced":
				op.merge = func(from, destination string) error {
					merged++
					if err := e.mergeArchiveDirectory(from, destination); err != nil {
						return err
					}
					if err := os.RemoveAll(candidate); err != nil {
						return err
					}
					archiveWrite(t, root, "a/assets/replacement", []byte("replacement"))
					return nil
				}
				op.removeAll = func(*os.Root, string) error { removed++; return nil }
			case "remove-failure":
				op.merge = func(from, destination string) error { merged++; return e.mergeArchiveDirectory(from, destination) }
				op.removeAll = func(*os.Root, string) error { removed++; return errors.New("resource remove failed") }
			}
			err := e.publishArchiveResourcesWithIO(root, "assets", op)
			if err == nil {
				t.Fatal("resource admission failure became success")
			}
			if kind == "walk-incomplete" && indexrepair.IsSourceNotExist(err) {
				t.Fatal("incomplete discovery admitted as absence", err)
			}
			if (kind == "walk-incomplete" || kind == "scan-replaced") && (merged != 0 || removed != 0) {
				t.Fatal("failed discovery admitted side effects", merged, removed)
			}
			if (kind == "merge-failure" || kind == "post-merge-replaced" || kind == "remove-failure") && merged != 1 {
				t.Fatal("candidate merge count", merged)
			}
			if kind != "remove-failure" && removed != 0 {
				t.Fatal("failed candidate removed", removed)
			}
			if kind == "remove-failure" && removed != 1 {
				t.Fatal("remove failure not exercised", removed)
			}
			if _, statErr := os.Stat(candidate); statErr != nil {
				t.Fatal("failed publication lost scratch", statErr)
			}
			if kind == "post-merge-replaced" || kind == "remove-failure" {
				raw, readErr := os.ReadFile(filepath.Join(util.DataDir, "assets/file"))
				if readErr != nil || string(raw) != "resource" {
					t.Fatal("successful merge prefix lost", string(raw), readErr)
				}
			}
		})
	}
}

func TestArchiveMergeCheckedManifestAndSequentialPublication(t *testing.T) {
	for _, kind := range []string{"walk-incomplete", "scan-replaced", "post-read-replaced", "later-read-incomplete", "final-manifest-replaced"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			first := archiveWrite(t, root, "a", []byte("first"))
			archiveWrite(t, root, "zz/last", []byte("last"))
			op := archiveMergeIO{}
			switch kind {
			case "walk-incomplete":
				sentinel := errors.New("merge walk incomplete")
				op.walk.readDir = func(ctx context.Context, source indexrepair.Source, current string) ([]os.DirEntry, error) {
					if current == "zz" {
						return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, sentinel)
					}
					return source.ReadDir(ctx, current)
				}
			case "scan-replaced":
				replaced := false
				op.walk.readDir = func(ctx context.Context, source indexrepair.Source, current string) ([]os.DirEntry, error) {
					if current == "zz" && !replaced {
						replaced = true
						if err := os.Remove(first); err != nil {
							t.Fatal(err)
						}
						archiveWrite(t, root, "a", []byte("replacement"))
					}
					return source.ReadDir(ctx, current)
				}
			case "post-read-replaced":
				op.read = func(ctx context.Context, source indexrepair.Source, current string) ([]byte, error) {
					raw, err := source.Read(ctx, current)
					if err == nil && current == "a" {
						if removeErr := os.Remove(first); removeErr != nil {
							t.Fatal(removeErr)
						}
						archiveWrite(t, root, "a", []byte("replacement"))
					}
					return raw, err
				}
			case "later-read-incomplete":
				op.read = func(ctx context.Context, source indexrepair.Source, current string) ([]byte, error) {
					if current == "zz/last" {
						return []byte("part"), errors.Join(indexrepair.ErrSourceReadIncomplete, errors.New("merge partial read"))
					}
					return source.Read(ctx, current)
				}
			case "final-manifest-replaced":
				op.read = func(ctx context.Context, source indexrepair.Source, current string) ([]byte, error) {
					raw, err := source.Read(ctx, current)
					if err == nil && current == "zz/last" {
						if removeErr := os.Remove(first); removeErr != nil {
							t.Fatal(removeErr)
						}
						archiveWrite(t, root, "a", []byte("replacement"))
					}
					return raw, err
				}
			}
			err := e.mergeArchiveDirectoryWithIO(root, "assets", op)
			if err == nil {
				t.Fatal("merge source failure became success")
			}
			publishedFirst, firstErr := os.ReadFile(filepath.Join(util.DataDir, "assets/a"))
			switch kind {
			case "walk-incomplete", "scan-replaced", "post-read-replaced":
				if !os.IsNotExist(firstErr) {
					t.Fatal("prepublication failure changed target", string(publishedFirst), firstErr)
				}
			case "later-read-incomplete", "final-manifest-replaced":
				if firstErr != nil || string(publishedFirst) != "first" {
					t.Fatal("sequential success prefix lost", string(publishedFirst), firstErr)
				}
			}
			if _, lastErr := os.Stat(filepath.Join(util.DataDir, "assets/zz/last")); kind != "final-manifest-replaced" && !os.IsNotExist(lastErr) {
				t.Fatal("failed later source was published", lastErr)
			}
			if kind == "walk-incomplete" && indexrepair.IsSourceNotExist(err) {
				t.Fatal("incomplete scan admitted as absence", err)
			}
		})
	}
}
func TestArchivePublicationSortPreservesMappingAndMissingSource(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	root := t.TempDir()
	box := "20261031100000-box0001"
	target := archiveWrite(t, util.DataDir, box+"/.siyuan/sort.json", []byte(`{"keep":3,"next":1}`))
	if err := e.mergeArchiveSort(root, box, nil); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(target)
	if string(before) != `{"keep":3,"next":1}` {
		t.Fatal("missing source rewrote target")
	}
	src := archiveWrite(t, root, ".siyuan/sort.json", []byte(`{"old":7,"stale":8}`))
	if err := e.mergeArchiveSort(root, box, map[string]string{"old": "next"}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(target)
	var rows map[string]int
	if err := json.Unmarshal(after, &rows); err != nil || !reflect.DeepEqual(rows, map[string]int{"keep": 3, "next": 7}) {
		t.Fatal(rows, err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("published sort source not removed", err)
	}
	// A new notebook sort file is created only when the archive actually has one.
	src = archiveWrite(t, root, ".siyuan/sort.json", []byte(`{"old":5}`))
	other := "20261031100001-box0002"
	if err := e.mergeArchiveSort(root, other, map[string]string{"old": "new"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(util.DataDir, other, ".siyuan/sort.json"))
	if !bytes.Contains(raw, []byte(`"new":5`)) {
		t.Fatal(string(raw))
	}
}
func TestArchivePublicationSortErrorsPreserveBothSources(t *testing.T) {
	for _, kind := range []string{"bad-import", "null-import", "bad-target", "null-target", "source-link", "target-link", "target-directory", "permission"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			box := "20261031100000-box0001"
			src := archiveWrite(t, root, ".siyuan/sort.json", []byte(`{"old":2}`))
			target := archiveWrite(t, util.DataDir, box+"/.siyuan/sort.json", []byte(`{"keep":1}`))
			outside := archiveWrite(t, t.TempDir(), "outside", []byte(`{"external":9}`))
			switch kind {
			case "bad-import":
				os.WriteFile(src, []byte("bad"), 0600)
			case "null-import":
				os.WriteFile(src, []byte("null"), 0600)
			case "bad-target":
				os.WriteFile(target, []byte("bad"), 0600)
			case "null-target":
				os.WriteFile(target, []byte("null"), 0600)
			case "source-link":
				os.Remove(src)
				os.Symlink(outside, src)
			case "target-link":
				os.Remove(target)
				os.Symlink(outside, target)
			case "target-directory":
				os.Remove(target)
				os.Mkdir(target, 0755)
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses mode bits")
				}
				os.Chmod(filepath.Dir(target), 0500)
				defer os.Chmod(filepath.Dir(target), 0755)
			}
			before, _ := os.ReadFile(target)
			sourceBefore, _ := os.ReadFile(src)
			if err := e.mergeArchiveSort(root, box, map[string]string{"old": "new"}); err == nil {
				t.Fatal("sort failure hidden", kind)
			}
			after, _ := os.ReadFile(target)
			sourceAfter, _ := os.ReadFile(src)
			if !bytes.Equal(before, after) || !bytes.Equal(sourceBefore, sourceAfter) {
				t.Fatal("failed sort modified input/target")
			}
			external, _ := os.ReadFile(outside)
			if string(external) != `{"external":9}` {
				t.Fatal("external modified")
			}
		})
	}
}
func TestArchivePublicationOwnerAndReadonlyRejectWrites(t *testing.T) {
	for _, kind := range []string{"cancel", "old-root", "closed", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			e, _, _ := archiveEntryFixture(t)
			root := t.TempDir()
			archiveWrite(t, root, "assets/a", []byte("incoming"))
			src := archiveWrite(t, root, ".siyuan/sort.json", []byte(`{"old":2}`))
			switch kind {
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				e.ctx = ctx
				e.binding.scope.ctx = ctx
				cancel()
			case "old-root":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "closed":
				if err := Conf.CloseCloudRepo(context.Background()); err != nil {
					t.Fatal(err)
				}
			case "readonly":
				Conf.m.Lock()
				Conf.persistenceReadOnly = true
				Conf.m.Unlock()
				defer func() { Conf.m.Lock(); Conf.persistenceReadOnly = false; Conf.m.Unlock() }()
			}
			if err := e.publishArchiveResources(root, "assets"); err == nil {
				t.Fatal("resource admitted")
			}
			if err := e.mergeArchiveSort(root, "20261031100000-box0001", map[string]string{"old": "new"}); err == nil {
				t.Fatal("sort admitted")
			}
			if _, err := os.Stat(src); err != nil {
				t.Fatal("rejected source removed")
			}
		})
	}
}
func TestArchivePublicationExternalRevisionIsPreserved(t *testing.T) {
	e, _, _ := archiveEntryFixture(t)
	file := archiveWrite(t, util.DataDir, "assets/a", []byte("admitted"))
	before, _ := os.ReadFile(file)
	io, closeIO, err := e.binding.scope.commit.r.openRepairIOChecked(e.ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer closeIO()
	os.WriteFile(file, []byte("external"), 0600)
	if err = e.rewriteArchiveFile(io.Source, "assets/a", before, []byte("incoming")); !errors.Is(err, indexrepair.ErrSourceChanged) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	if string(after) != "external" {
		t.Fatal("new source overwritten")
	}
}
