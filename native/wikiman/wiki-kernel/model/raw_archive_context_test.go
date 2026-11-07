//go:build fts5

package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/util"
)

func TestRawArchiveDataResourcesAndDesktop(t *testing.T) {
	_, _, p, _ := restoreFixture(t)
	archiveWrite(t, p.Data, "assets/file.txt", []byte("original"))
	if e := os.MkdirAll(filepath.Join(p.Data, "empty"), 0700); e != nil {
		t.Fatal(e)
	}
	u, e := ExportDataContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	entries := readOutputZip(t, filepath.Join(p.Temp, "export", filepath.Base(u)))
	found, empty := false, false
	for name, raw := range entries {
		if strings.HasSuffix(name, "/assets/file.txt") {
			found = string(raw) == "original"
		}
		if strings.HasSuffix(name, "/empty/") {
			empty = true
		}
	}
	if !found || !empty {
		t.Fatal(entries)
	}
	u, e = ExportResourcesContext(context.Background(), []string{"data/assets/file.txt"}, "bundle")
	if e != nil {
		t.Fatal(e)
	}
	entries = readOutputZip(t, filepath.Join(p.Temp, "export", filepath.Base(u)))
	if string(entries["bundle/file.txt"]) != "original" {
		t.Fatal(entries)
	}
	destination := t.TempDir()
	name, e := ExportDataInFolderContext(context.Background(), destination)
	if e != nil {
		t.Fatal(e)
	}
	if len(readOutputZip(t, filepath.Join(destination, name))) == 0 {
		t.Fatal("empty desktop zip")
	}
	if _, e = os.Stat(filepath.Join(p.Temp, "export", name)); !os.IsNotExist(e) {
		t.Fatal("desktop staging retained", e)
	}
	if _, e = ExportDataInFolderContext(context.Background(), p.Data); e == nil {
		t.Fatal("source destination admitted")
	}
	raw, e := os.ReadFile(filepath.Join(p.Data, "assets/file.txt"))
	if e != nil || string(raw) != "original" {
		t.Fatal("source changed", e)
	}
}

func TestRawArchiveFailuresAndCancellationDoNotPublish(t *testing.T) {
	_, _, p, _ := restoreFixture(t)
	outside := archiveWrite(t, t.TempDir(), "private", []byte("outside"))
	if e := os.Symlink(outside, filepath.Join(p.Data, "linked")); e != nil {
		t.Fatal(e)
	}
	if u, e := ExportDataContext(context.Background()); e == nil || u != "" {
		t.Fatal(u, e)
	}
	for _, paths := range [][]string{{"../outside"}, {"data/linked"}, {"data/missing"}, {"temp"}} {
		if u, e := ExportResourcesContext(context.Background(), paths, "bundle"); e == nil || u != "" {
			t.Fatal(paths, u, e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if u, e := ExportDataContext(ctx); !errors.Is(e, context.Canceled) || u != "" {
		t.Fatal(u, e)
	}
	entries, e := os.ReadDir(filepath.Join(p.Temp, "export"))
	if e != nil && !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if len(entries) != 0 {
		t.Fatal("failed archive left publication", entries)
	}
}

func TestRawArchiveLogsSelectedDeviceAndMissingOptional(t *testing.T) {
	_, _, p, _ := restoreFixture(t)
	old := util.HomeDir
	util.HomeDir = t.TempDir()
	defer func() { util.HomeDir = old }()
	archiveWrite(t, util.HomeDir, ".config/siyuan/app.log", []byte("app"))
	archiveWrite(t, p.Temp, "kernel-not-selected.log", []byte("private"))
	archiveWrite(t, p.Temp, "mobile.log", []byte("mobile"))
	u, e := ExportSystemLogContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	entries := readOutputZip(t, filepath.Join(p.Temp, "export", filepath.Base(u)))
	if len(entries) != 2 || string(entries["log/app.log"]) != "app" || string(entries["log/mobile.log"]) != "mobile" {
		t.Fatal(entries)
	}
}

func TestWorkspaceFinalizationIsJoinedByOwnerClose(t *testing.T) {
	c, _, _, _ := restoreFixture(t)
	base, e := captureLegacyReindexOwner(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- base.runFinalized(context.Background(), func(context.Context, context.Context) error { return nil }, func(error) error { close(entered); <-release; return errors.New("cleanup sentinel") })
	}()
	<-entered
	closed := make(chan struct{})
	go func() { c.CloseCloudRepo(context.Background()); close(closed) }()
	select {
	case <-closed:
		t.Fatal("close detached cleanup")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if e := <-done; e == nil || !strings.Contains(e.Error(), "cleanup sentinel") {
		t.Fatal(e)
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("close did not join finalizer")
	}
}
