//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func TestArchiveExportCloseBorrowCapturesBackendAndPreservesOwner(t *testing.T) {
	e, o := outputFixture(t)
	sentinel := errors.New("export target close failed")
	calls := 0
	e.closeRoot = func(root *os.Root) error { calls++; return errors.Join(root.Close(), sentinel) }
	root, closeIO, err := e.openExportIO(o.folder)
	if err != nil {
		t.Fatal(err)
	}
	e.closeRoot = func(*os.Root) error { t.Fatal("close followed later binding"); return nil }
	for i := 0; i < 2; i++ {
		if err = closeIO(); !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("borrow closed more than once", calls)
	}
	if _, err = root.Stat("."); err == nil {
		t.Fatal("borrowed descriptor still open")
	}
	if _, err = o.payload.Stat("."); err != nil {
		t.Fatal("borrow closed output owner", err)
	}
}
func TestArchiveExportCloseDocumentResourcesAndSortFailures(t *testing.T) {
	for _, kind := range []string{"document", "resources", "sort", "sort-empty"} {
		t.Run(kind, func(t *testing.T) {
			e, box, file := exportContextFixture(t)
			before, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			folder := t.TempDir()
			id := util.GetTreeID(file)
			archiveWrite(t, e.binding.scope.commit.r.data, box+"/.siyuan/sort.json", []byte(`{"`+id+`":17,"stale":6}`))
			sentinel := errors.New("target close failed")
			closed := 0
			e.closeRoot = func(root *os.Root) error { closed++; return errors.Join(root.Close(), sentinel) }
			switch kind {
			case "document":
				err = e.writeExportBytes(folder, id+".docj", before)
				if got, _ := os.ReadFile(filepath.Join(folder, id+".docj")); !bytes.Equal(got, before) {
					t.Fatal("late close implied rollback")
				}
			case "resources":
				archiveWrite(t, e.binding.scope.commit.r.data, "assets/exported.txt", []byte("resource bytes"))
				tree := exportReferenceDoc(t, e, box, ast.NewNodeID(), resourceLink("assets/exported.txt"))
				err = e.exportResources(folder, map[string]*parse.Tree{tree.ID: tree})
				if got, _ := os.ReadFile(filepath.Join(folder, "assets/exported.txt")); string(got) != "resource bytes" {
					t.Fatal("late close implied resource rollback")
				}
			case "sort":
				err = e.writeSort(folder, box, map[string]*parse.Tree{id: nil})
				raw, readErr := os.ReadFile(filepath.Join(folder, ".siyuan/sort.json"))
				var got map[string]int
				if readErr != nil || json.Unmarshal(raw, &got) != nil || len(got) != 1 || got[id] != 17 {
					t.Fatal("sort projection changed", string(raw), readErr)
				}
			case "sort-empty":
				archiveWrite(t, folder, ".siyuan/sort.json", []byte(`{"old":1}`))
				err = e.writeSort(folder, box, nil)
				if _, statErr := os.Stat(filepath.Join(folder, ".siyuan/sort.json")); !os.IsNotExist(statErr) {
					t.Fatal("empty sort not removed", statErr)
				}
			}
			if !errors.Is(err, sentinel) || closed != 1 {
				t.Fatal("close failure swallowed", err, closed)
			}
			if got, _ := os.ReadFile(file); !bytes.Equal(got, before) {
				t.Fatal("export changed native source")
			}
		})
	}
}
func TestArchiveExportCloseJoinsPrimarySourceAndTargetFailures(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	targetClose, sourceClose := errors.New("target close"), errors.New("source close")
	e.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), targetClose) }
	folder := t.TempDir()
	err := e.writeExportBytes(folder, "../outside", []byte("must not escape"))
	if !errors.Is(err, indexrepair.ErrSourcePath) || !errors.Is(err, targetClose) {
		t.Fatal(err)
	}
	archiveWrite(t, e.binding.scope.commit.r.data, "assets/close.txt", []byte("source"))
	tree := exportReferenceDoc(t, e, box, ast.NewNodeID(), resourceLink("assets/close.txt"))
	e.binding.scope.commit.r.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), sourceClose) }
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(folder, "assets")); err != nil {
		t.Fatal(err)
	}
	err = e.exportResources(folder, map[string]*parse.Tree{tree.ID: tree})
	if !errors.Is(err, indexrepair.ErrSourcePath) || !errors.Is(err, targetClose) || !errors.Is(err, sourceClose) {
		t.Fatal("causes lost", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("followed output link")
	}
}
func TestArchiveExportCloseFailurePreventsArchivePublicationAndCleansScratch(t *testing.T) {
	e, _, file := exportContextFixture(t)
	selected, err := e.single(util.GetTreeID(file))
	if err != nil {
		t.Fatal(err)
	}
	e.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), errors.New("target close failed")) }
	result := exportSYZipWithContext(e, selected.box, selected.rootDir, selected.folder, selected.paths)
	if result != "" {
		t.Fatal("failed close published success URL", result)
	}
	if e.output != nil {
		t.Fatal("failed export retained output owner")
	}
	entries, err := os.ReadDir(e.temp)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "export" {
			t.Fatal("private staging leaked", entry.Name())
		}
	}
	published, err := os.ReadDir(filepath.Join(e.temp, "export"))
	if err != nil || len(published) != 0 {
		t.Fatal("failed export published ZIP", published, err)
	}
}
func TestArchiveExportCloseCanceledAndUnboundBorrowRejected(t *testing.T) {
	var unbound *archiveExportContext
	if _, _, err := unbound.openExportIO(t.TempDir()); err == nil {
		t.Fatal("unbound borrow accepted")
	}
	e, _, _ := exportContextFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.binding.scope.ctx = ctx
	e.closeRoot = func(*os.Root) error { t.Fatal("canceled operation borrowed root"); return nil }
	if _, _, err := e.openExportIO(t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
