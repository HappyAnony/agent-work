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
	"testing"
	"time"

	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func dataImportFixture(t *testing.T) (*DataImportContext, string, []byte, *int) {
	t.Helper()
	e, file, before := archiveEntryFixture(t)
	completed := new(int)
	d := &DataImportContext{archive: e, complete: func() error {
		e.scratchMu.Lock()
		count := len(e.scratch)
		e.scratchMu.Unlock()
		if count != 0 {
			t.Error("completion before owned scratch cleanup", count)
		}
		*completed++
		return nil
	}}
	return d, file, before, completed
}
func assertDataImportClean(t *testing.T, d *DataImportContext) {
	t.Helper()
	e := d.archive
	e.scratchMu.Lock()
	count := len(e.scratch)
	e.scratchMu.Unlock()
	if count != 0 {
		t.Fatal("scratch registry leaked", count)
	}
	entries, err := os.ReadDir(e.temp)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "wiki-data-import-") || strings.HasPrefix(entry.Name(), "wiki-extract-") {
			t.Fatal("owned scratch leaked", entry.Name())
		}
	}
	if e.binding.scope.commit.r.owner.syncRuntime.running.Load() {
		t.Fatal("owner import lock left running")
	}
}
func TestDataImportOriginalMergeIDsMetadataAndCleanup(t *testing.T) {
	d, file, before, completed := dataImportFixture(t)
	e := d.archive
	data := e.binding.scope.commit.r.data
	box := filepath.Base(filepath.Dir(file))
	root := "data-20261031120000/"
	expected := bytes.ReplaceAll(before, []byte("changed title"), []byte("Imported original ID"))
	// Keep byte representation and IDs; complete data imports never remap AVs/docs.
	members := []archiveMember{{root + box + "/" + filepath.Base(file), expected, 0}, {root + box + "/.siyuan/conf.json", []byte(`{"name":"Imported notebook","sort":2}`), 0}, {root + "storage/av/20261029150000-deepav1.json", []byte(`{"id":"20261029150000-deepav1","opaque":"retained"}`), 0}, {root + "assets/same.txt", []byte("new asset"), 0}, {root + "assets/empty/", nil, os.ModeDir | 0755}, {root + "emojis/bad?.png", []byte("emoji"), 0}}
	archiveWrite(t, data, "assets/same.txt", []byte("old asset"))
	untouched := archiveWrite(t, data, "assets/untouched.txt", []byte("retained"))
	zipPath := makeArchive(t, members...)
	input, _ := os.ReadFile(zipPath)
	if err := d.ImportDataReader(bytes.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	if *completed != 1 {
		t.Fatal("completion count", *completed)
	}
	for name, want := range map[string][]byte{box + "/" + filepath.Base(file): expected, box + "/.siyuan/conf.json": members[1].data, "storage/av/20261029150000-deepav1.json": members[2].data, "assets/same.txt": []byte("new asset"), "emojis/" + util.FilterUploadEmojiFileName("bad?.png"): []byte("emoji")} {
		got, err := os.ReadFile(filepath.Join(data, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("merge/identity changed", name, err)
		}
	}
	if b, _ := os.ReadFile(untouched); string(b) != "retained" {
		t.Fatal("merge deleted unrelated data")
	}
	if st, err := os.Stat(filepath.Join(data, "assets/empty")); err != nil || !st.IsDir() {
		t.Fatal("empty directory lost", err)
	}
	if after, _ := os.ReadFile(zipPath); !bytes.Equal(input, after) {
		t.Fatal("caller archive changed")
	}
	assertDataImportClean(t, d)
}
func TestDataImportRejectsInvalidArchiveBeforeSourceWrites(t *testing.T) {
	for _, kind := range []string{"document", "traversal", "symlink", "multi-root", "emoji-collision", "crc"} {
		t.Run(kind, func(t *testing.T) {
			d, file, before, completed := dataImportFixture(t)
			members := []archiveMember{{"Root/assets/would-write", []byte("no"), 0}}
			switch kind {
			case "document":
				members = append(members, archiveMember{"Root/20261031120000-doc0001.docj", before, 0})
			case "traversal":
				members = append(members, archiveMember{"Root/../escape", nil, 0})
			case "symlink":
				members = append(members, archiveMember{"Root/link", []byte("outside"), os.ModeSymlink | 0777})
			case "multi-root":
				members = append(members, archiveMember{"Other/file", nil, 0})
			case "emoji-collision":
				members = append(members, archiveMember{"Root/emojis/bad?.png", nil, 0}, archiveMember{"Root/emojis/" + util.FilterUploadEmojiFileName("bad?.png"), nil, 0})
			case "crc":
				members[0].data = []byte("unique-crc-payload")
			}
			zipPath := makeArchive(t, members...)
			if kind == "crc" {
				raw, _ := os.ReadFile(zipPath)
				i := bytes.Index(raw, []byte("unique-crc-payload"))
				if i < 0 {
					t.Fatal("fixture payload")
				}
				raw[i] ^= 1
				os.WriteFile(zipPath, raw, 0600)
			}
			if err := d.ImportFile(zipPath); err == nil {
				t.Fatal("invalid input accepted")
			}
			if *completed != 0 {
				t.Fatal("invalid input scheduled completion")
			}
			if raw, _ := os.ReadFile(file); !bytes.Equal(raw, before) {
				t.Fatal("invalid input changed source")
			}
			if _, err := os.Stat(filepath.Join(d.archive.binding.scope.commit.r.data, "assets/would-write")); !os.IsNotExist(err) {
				t.Fatal("preflight wrote destination", err)
			}
			assertDataImportClean(t, d)
		})
	}
}
func TestDataImportDocumentEntryStillRejectsFullDataZIP(t *testing.T) {
	d, _, _, _ := dataImportFixture(t)
	zipPath := makeArchive(t, archiveMember{"data-20261031120000/assets/file", []byte("data"), 0})
	if _, _, err := d.archive.extract(zipPath); err == nil {
		t.Fatal("document entry accepted full data ZIP")
	}
	if err := d.ImportFile(zipPath); err != nil {
		t.Fatal("data entry rejected full data ZIP", err)
	}
	assertDataImportClean(t, d)
}
func TestDataImportCanceledUploadAndStaleOwnerDoNotComplete(t *testing.T) {
	for _, kind := range []string{"cancel", "read-error", "owner", "temp", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			d, file, before, completed := dataImportFixture(t)
			sentinel := errors.New("upload failed")
			var reader io.Reader = bytes.NewReader([]byte("zip"))
			switch kind {
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.archive.ctx = ctx
			case "read-error":
				reader = dataImportFailReader{sentinel}
			case "owner":
				old := Conf
				Conf = NewAppConf()
				defer func() { Conf = old }()
			case "temp":
				old := util.TempDir
				util.TempDir = t.TempDir()
				defer func() { util.TempDir = old }()
			case "readonly":
				c := d.archive.binding.scope.commit.r.owner
				c.m.Lock()
				c.persistenceReadOnly = true
				c.m.Unlock()
				defer func() { c.m.Lock(); c.persistenceReadOnly = false; c.m.Unlock() }()
			}
			err := d.ImportDataReader(reader)
			if err == nil || *completed != 0 {
				t.Fatal(err, *completed)
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if kind == "read-error" && !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			if raw, _ := os.ReadFile(file); !bytes.Equal(raw, before) {
				t.Fatal("failed admission changed source")
			}
			assertDataImportClean(t, d)
		})
	}
}

type dataImportFailReader struct{ err error }

func (r dataImportFailReader) Read([]byte) (int, error) { return 0, r.err }
func TestDataImportLateCloseFailureReportsPartialWriteWithoutCompletion(t *testing.T) {
	d, _, _, completed := dataImportFixture(t)
	sentinel := errors.New("data target close failed")
	e := d.archive
	e.closeRoot = func(root *os.Root) error {
		name := root.Name()
		err := root.Close()
		if filepath.Clean(name) == filepath.Clean(e.binding.scope.commit.r.data) {
			return errors.Join(err, sentinel)
		}
		return err
	}
	zipPath := makeArchive(t, archiveMember{"Root/assets/late-close.txt", []byte("already published"), 0})
	err := d.ImportFile(zipPath)
	if !errors.Is(err, sentinel) || *completed != 0 {
		t.Fatal(err, *completed)
	}
	if raw, _ := os.ReadFile(filepath.Join(e.binding.scope.commit.r.data, "assets/late-close.txt")); string(raw) != "already published" {
		t.Fatal("close failure implied rollback")
	}
	assertDataImportClean(t, d)
}
func TestDataImportDestinationSymlinkAndShutdownRejectCompletion(t *testing.T) {
	for _, kind := range []string{"symlink", "closed", "admission-error"} {
		t.Run(kind, func(t *testing.T) {
			d, _, _, completed := dataImportFixture(t)
			e := d.archive
			outside := t.TempDir()
			if kind == "symlink" {
				if err := os.Symlink(outside, filepath.Join(e.binding.scope.commit.r.data, "linked")); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "closed" {
				c := e.binding.scope.commit.r.owner
				c.repoMu.Lock()
				c.repoClosed = true
				c.repoMu.Unlock()
			}
			sentinel := errors.New("index admission failed")
			if kind == "admission-error" {
				d.complete = func() error { return sentinel }
			}
			zipPath := makeArchive(t, archiveMember{"Root/linked/file", []byte("incoming"), 0})
			err := d.ImportFile(zipPath)
			if err == nil || *completed != 0 {
				t.Fatal(err, *completed)
			}
			if kind == "admission-error" && !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(outside, "file")); !os.IsNotExist(err) {
				t.Fatal("wrote outside captured source", err)
			}
			assertDataImportClean(t, d)
		})
	}
}
func TestDataImportReindexAdmissionRejectsStoppedCanceledAndUnauthorized(t *testing.T) {
	c, _, _, _, _ := referenceModelFixture(t)
	c.repoTasks = taskgroup.Open()
	c.repoTasks.Stop()
	defer c.repoTasks.Close(context.Background())
	before := time.Now().Add(time.Hour)
	c.syncRuntime.same.Store(7)
	c.syncRuntime.planMu.Lock()
	c.syncRuntime.plan = before
	c.syncRuntime.planMu.Unlock()
	for _, kind := range []string{"stopped", "cancel", "unauthorized"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancel" {
				cancel()
			}
			sentinel := errors.New("source owner changed")
			err := c.admitFullNotebookReindex(ctx, func() error {
				if kind == "unauthorized" {
					return sentinel
				}
				return nil
			}, func() { t.Error("failed admission ran success effects") })
			if err == nil {
				t.Fatal("rebuild unexpectedly admitted")
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if kind == "unauthorized" && !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
		})
	}
	c.syncRuntime.planMu.Lock()
	after := c.syncRuntime.plan
	c.syncRuntime.planMu.Unlock()
	if after != before || c.syncRuntime.same.Load() != 7 {
		t.Fatal("failed admission changed sync")
	}
}

func TestFullReindexContextRejectsCanceledAdmission(t *testing.T) {
	c, _, _, _, _ := referenceModelFixture(t)
	previous := Conf
	Conf = c
	t.Cleanup(func() { Conf = previous })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := FullReindexContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
