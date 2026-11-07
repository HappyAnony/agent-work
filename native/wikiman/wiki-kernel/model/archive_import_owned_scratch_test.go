//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/util"
)

func ownedImportFixture(t *testing.T) (*ArchiveImportContext, string) {
	t.Helper()
	e, source, _ := archiveEntryFixture(t)
	return e, filepath.Base(filepath.Dir(source))
}
func assertNoImportScratch(t *testing.T, e *ArchiveImportContext) {
	t.Helper()
	e.scratchMu.Lock()
	n := len(e.scratch)
	e.scratchMu.Unlock()
	if n != 0 {
		t.Fatal("live scratch owners remain", n)
	}
	for _, prefix := range []string{"wiki-import-*", "wiki-extract-*"} {
		names, _ := filepath.Glob(filepath.Join(e.temp, prefix))
		if len(names) != 0 {
			t.Fatal("scratch residue", names)
		}
	}
}
func TestArchiveImportOwnedPreparationUsesTempNotSourceParent(t *testing.T) {
	e, _ := ownedImportFixture(t)
	id := ast.NewNodeID()
	file := makeArchive(t, archiveMember{"Root/" + id + ".docj", mountDoc(id, "Imported"), 0})
	before, _ := os.ReadFile(file)
	p, err := e.prepare(file)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p.scratch) != e.temp || filepath.Dir(p.scratch) == filepath.Dir(file) {
		t.Fatal("extraction created beside caller archive")
	}
	if len(p.trees) != 1 {
		t.Fatal("original preparation lost")
	}
	if err = p.close(); err != nil {
		t.Fatal(err)
	}
	assertNoImportScratch(t, e)
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("caller ZIP changed")
	}
}
func TestArchiveImportOwnedBadSourceCleanupAndCRC(t *testing.T) {
	for _, kind := range []string{"bad-doc", "crc", "two-roots"} {
		t.Run(kind, func(t *testing.T) {
			e, _ := ownedImportFixture(t)
			id := ast.NewNodeID()
			raw := mountDoc(id, "Fixture")
			members := []archiveMember{{"Root/" + id + ".docj", raw, 0}}
			if kind == "bad-doc" {
				members[0].data = []byte("bad")
			}
			if kind == "two-roots" {
				members = append(members, archiveMember{"Other/file", []byte("other"), 0})
			}
			file := makeArchive(t, members...)
			if kind == "crc" {
				body, _ := os.ReadFile(file)
				at := bytes.Index(body, raw)
				if at < 0 {
					t.Fatal("CRC fixture missing")
				}
				body[at] ^= 1
				os.WriteFile(file, body, 0600)
			}
			if p, err := e.prepare(file); err == nil {
				p.close()
				t.Fatal("bad archive accepted", kind)
			}
			assertNoImportScratch(t, e)
		})
	}
}

type ownedImportFaultReader struct {
	e            *ArchiveImportContext
	kind, folder string
	done         bool
}

func (r *ownedImportFaultReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	r.e.scratchMu.Lock()
	var d *archiveImportScratch
	for _, entry := range r.e.scratch {
		if strings.HasPrefix(entry.name, "wiki-import-") {
			d = entry
			break
		}
	}
	r.e.scratchMu.Unlock()
	if d == nil {
		return 0, errors.New("upload owner missing")
	}
	r.folder = d.folder
	if r.kind == "permission" {
		d.root.WriteFile("retained", []byte("body"), 0600)
		os.Chmod(d.folder, 0000)
	} else {
		os.Rename(d.folder, d.folder+"-old")
		os.Mkdir(d.folder, 0700)
		os.WriteFile(filepath.Join(d.folder, "keep"), []byte("replacement"), 0600)
	}
	copy(p, "partial")
	return 7, io.ErrUnexpectedEOF
}
func TestArchiveImportOwnedUploadJoinsPrimaryAndCleanupErrors(t *testing.T) {
	for _, kind := range []string{"permission", "replacement"} {
		t.Run(kind, func(t *testing.T) {
			if kind == "permission" && os.Geteuid() == 0 {
				t.Skip("root bypasses permissions")
			}
			e, box := ownedImportFixture(t)
			reader := &ownedImportFaultReader{e: e, kind: kind}
			err := e.ImportReader(reader, box, "/")
			if reader.folder != "" {
				defer func() {
					os.Chmod(reader.folder, 0700)
					os.RemoveAll(reader.folder)
					os.RemoveAll(reader.folder + "-old")
				}()
			}
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal("primary upload error lost", err)
			}
			if kind == "permission" && !errors.Is(err, os.ErrPermission) {
				t.Fatal("cleanup permission error lost", err)
			}
			if kind == "replacement" {
				raw, _ := os.ReadFile(filepath.Join(reader.folder, "keep"))
				if string(raw) != "replacement" || !strings.Contains(err.Error(), "owner_changed") {
					t.Fatal("replacement removed or cleanup error hidden", err)
				}
			}
		})
	}
}
func TestArchiveImportOwnedCanceledOrChangedTempCleansCapturedRoot(t *testing.T) {
	for _, kind := range []string{"cancel", "temp-switch", "parent-replaced"} {
		t.Run(kind, func(t *testing.T) {
			e, _ := ownedImportFixture(t)
			d, err := e.newImportScratch("wiki-import-")
			if err != nil {
				t.Fatal(err)
			}
			d.root.WriteFile("file", []byte("own"), 0600)
			oldTemp := e.temp
			switch kind {
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				e.ctx = ctx
				e.binding.scope.ctx = ctx
				cancel()
			case "temp-switch":
				old := util.TempDir
				util.TempDir = t.TempDir()
				defer func() { util.TempDir = old }()
			case "parent-replaced":
				os.Rename(oldTemp, oldTemp+"-old")
				os.Mkdir(oldTemp, 0700)
				os.Mkdir(filepath.Join(oldTemp, d.name), 0700)
				os.WriteFile(filepath.Join(oldTemp, d.name, "keep"), []byte("foreign"), 0600)
			}
			if err = e.guard(); err == nil {
				t.Fatal("invalid owner accepted", kind)
			}
			if err = d.close(); err != nil {
				t.Fatal("captured cleanup failed", err)
			}
			if kind == "parent-replaced" {
				raw, _ := os.ReadFile(filepath.Join(oldTemp, d.name, "keep"))
				if string(raw) != "foreign" {
					t.Fatal("new temp content deleted")
				}
				if _, err = os.Stat(filepath.Join(oldTemp+"-old", d.name)); !os.IsNotExist(err) {
					t.Fatal("old owner residue", err)
				}
			} else {
				if _, err = os.Stat(d.folder); !os.IsNotExist(err) {
					t.Fatal("scratch not removed", err)
				}
			}
		})
	}
}
func TestArchiveImportOwnedPreparedCloseReportsFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permissions")
	}
	e, _ := ownedImportFixture(t)
	id := ast.NewNodeID()
	p, err := e.prepare(makeArchive(t, archiveMember{"Root/" + id + ".docj", mountDoc(id, "Doc"), 0}))
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(p.root, 0000)
	defer func() { os.Chmod(p.root, 0700); os.RemoveAll(p.scratch) }()
	if err = p.close(); !errors.Is(err, os.ErrPermission) {
		t.Fatal("prepared cleanup failure hidden", err)
	}
	if again := p.close(); !errors.Is(again, os.ErrPermission) {
		t.Fatal("cleanup failure receipt lost", again)
	}
}
func TestArchiveImportOwnedInputLinksAndReplacedRootReject(t *testing.T) {
	e, _ := ownedImportFixture(t)
	d, err := e.newImportScratch("wiki-import-")
	if err != nil {
		t.Fatal(err)
	}
	defer d.close()
	file := makeArchive(t, archiveMember{"Root/file", []byte("body"), 0})
	d.root.Symlink(file, "archive.zip")
	if _, err = e.openImportArchive(filepath.Join(d.folder, "archive.zip")); err == nil {
		t.Fatal("owned input link followed")
	}
	external := filepath.Join(t.TempDir(), "alias.zip")
	os.Symlink(file, external)
	if _, err = e.openImportArchive(external); err == nil {
		t.Fatal("external input link followed")
	}
	os.Rename(d.folder, d.folder+"-old")
	os.Mkdir(d.folder, 0700)
	os.WriteFile(filepath.Join(d.folder, "keep"), []byte("foreign"), 0600)
	if _, err = e.openImportArchive(filepath.Join(d.folder, "archive.zip")); err == nil {
		t.Fatal("replaced input owner accepted")
	}
	if err = d.close(); err == nil {
		t.Fatal("replacement cleanup accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(d.folder, "keep"))
	if string(raw) != "foreign" {
		t.Fatal("replacement deleted")
	}
}
func TestArchiveImportOwnedTempLinkAndOutsideWorkspaceReject(t *testing.T) {
	for _, kind := range []string{"link", "outside"} {
		t.Run(kind, func(t *testing.T) {
			e, _ := ownedImportFixture(t)
			outside := t.TempDir()
			os.WriteFile(filepath.Join(outside, "keep"), []byte("foreign"), 0600)
			if kind == "link" {
				os.MkdirAll(filepath.Dir(e.temp), 0700)
				os.RemoveAll(e.temp)
				os.Symlink(outside, e.temp)
			} else {
				old := util.TempDir
				util.TempDir = outside
				e.temp = outside
				defer func() { util.TempDir = old }()
			}
			if d, err := e.newImportScratch("wiki-import-"); err == nil {
				d.close()
				t.Fatal("invalid temp accepted", kind)
			}
			raw, _ := os.ReadFile(filepath.Join(outside, "keep"))
			if string(raw) != "foreign" {
				t.Fatal("outside changed")
			}
		})
	}
}
