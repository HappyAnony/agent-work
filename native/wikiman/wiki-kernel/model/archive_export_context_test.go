//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func exportContextFixture(t *testing.T) (*archiveExportContext, string, string) {
	t.Helper()
	_, source, _ := archiveEntryFixture(t)
	e, err := captureArchiveExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return e, filepath.Base(filepath.Dir(source)), source
}
func TestArchiveExportContextSingleNotebookAndMetadataSnapshot(t *testing.T) {
	e, box, source := exportContextFixture(t)
	rootID := util.GetTreeID(source)
	child, grand := ast.NewNodeID(), ast.NewNodeID()
	archiveWrite(t, util.DataDir, box+"/"+rootID+"/"+child+".docj", mountDoc(child, "Child"))
	archiveWrite(t, util.DataDir, box+"/"+rootID+"/"+child+"/"+grand+".docj", mountDoc(grand, "Grand"))
	archiveWrite(t, util.DataDir, box+"/assets/"+ast.NewNodeID()+".docj", mountDoc(ast.NewNodeID(), "Ignored"))
	archiveWrite(t, util.DataDir, box+"/.hidden/file.docj", []byte("ignore"))
	tmp := archiveWrite(t, util.DataDir, box+"/old.tmp", []byte("keep"))
	past := time.Now().Add(-time.Hour)
	os.Chtimes(tmp, past, past)
	selected, err := e.single(rootID)
	if err != nil || selected.box != box || selected.name != rootID || selected.rootDir != "/" {
		t.Fatal(selected, err)
	}
	want := []string{"/" + rootID + ".docj", "/" + rootID + "/" + child + "/" + grand + ".docj", "/" + rootID + "/" + child + ".docj"}
	if !reflect.DeepEqual(selected.paths, want) {
		t.Fatal(selected.paths, want)
	}
	notebook, err := e.notebook(box)
	if err != nil || len(notebook.paths) != 4 {
		t.Fatal(notebook, err)
	}
	if _, err = os.Stat(tmp); err != nil {
		t.Fatal("export selection removed source tmp", err)
	}
	old := e.boxes[box]
	Conf.business = conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) {
		return []conf.Notebook{{ID: box, Name: "Changed", Closed: true}}, nil
	}, nil, nil)
	frozen, err := e.box(box)
	if err != nil || frozen.Name != old.Name || frozen.Closed {
		t.Fatal("metadata followed later global state", frozen, err)
	}
}
func TestArchiveExportContextSelectedTreesReadonlyAndFailurePropagation(t *testing.T) {
	e, box, source := exportContextFixture(t)
	id := util.GetTreeID(source)
	before, _ := os.ReadFile(source)
	var raw map[string]interface{}
	json.Unmarshal(before, &raw)
	delete(raw, "Spec")
	legacy, _ := json.Marshal(raw)
	os.WriteFile(source, legacy, 0600)
	trees, err := e.selectedTrees(box, []string{"/" + id + ".docj", "ignored.txt"})
	if err != nil || len(trees) != 1 || trees[id].ID != id || trees[id].Root.Spec != "1" {
		t.Fatal(trees, err)
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(legacy, after) {
		t.Fatal("export reader repaired source")
	}
	os.WriteFile(source, []byte("bad JSON"), 0600)
	e, err = captureArchiveExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.selectedTrees(box, []string{"/" + id + ".docj"}); err == nil {
		t.Fatal("bad selected source skipped")
	}
	os.Remove(source)
	if _, err = e.selectedTrees(box, []string{"/" + id + ".docj"}); !os.IsNotExist(err) {
		t.Fatal("missing selected source skipped", err)
	}
}
func TestArchiveExportContextMissingClosedAndRegistryFailure(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	if _, err := e.single(ast.NewNodeID()); err == nil {
		t.Fatal("missing block selected")
	}
	if _, err := e.notebook(ast.NewNodeID()); err == nil {
		t.Fatal("missing notebook selected")
	}
	r := e.binding.scope.commit.r
	if _, err := r.registry.DB.Exec("ALTER TABLE blocktrees RENAME TO hidden_export_probe"); err != nil {
		t.Fatal(err)
	}
	defer r.registry.DB.Exec("ALTER TABLE hidden_export_probe RENAME TO blocktrees")
	if _, err := e.single(ast.NewNodeID()); err == nil {
		t.Fatal("actual lookup SQL failure swallowed")
	}
	Conf.business = conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) { return []conf.Notebook{{ID: box, Closed: true}}, nil }, nil, nil)
	closed, err := captureArchiveExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = closed.notebook(box); err == nil {
		t.Fatal("closed notebook selected")
	}
	Conf.business = conf.NewBusiness(func(context.Context) ([]conf.Notebook, error) { return nil, errors.New("notebook source failed") }, nil, nil)
	if _, err = captureArchiveExport(context.Background()); err == nil {
		t.Fatal("notebook dependency failure swallowed")
	}
}
func TestArchiveExportContextScanErrorsAndReservedFiltering(t *testing.T) {
	for _, kind := range []string{"link", "permission", "reserved-link", "missing-child"} {
		t.Run(kind, func(t *testing.T) {
			e, box, source := exportContextFixture(t)
			id := util.GetTreeID(source)
			switch kind {
			case "link":
				os.Symlink(t.TempDir(), filepath.Join(util.DataDir, box, "linked"))
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				dir := filepath.Join(util.DataDir, box, "denied")
				os.Mkdir(dir, 0000)
				defer os.Chmod(dir, 0755)
			case "reserved-link":
				os.Symlink(t.TempDir(), filepath.Join(util.DataDir, box, "assets"))
			}
			if kind == "missing-child" {
				selected, err := e.single(id)
				if err != nil || len(selected.paths) != 1 {
					t.Fatal("missing optional child directory", selected, err)
				}
				return
			}
			_, err := e.notebook(box)
			if kind == "reserved-link" {
				if err != nil {
					t.Fatal("reserved subtree followed", err)
				}
			} else if err == nil {
				t.Fatal("scan error hidden", kind)
			}
		})
	}
}
func TestArchiveExportContextSortProjectionAndStaleOutputRemoval(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	folder := t.TempDir()
	id := ast.NewNodeID()
	archiveWrite(t, util.DataDir, box+"/.siyuan/sort.json", []byte(`{"`+id+`":7,"unselected":3}`))
	target := archiveWrite(t, folder, ".siyuan/sort.json", []byte(`{"stale":99}`))
	trees := map[string]*parse.Tree{id: {ID: id}}
	if err := e.writeSort(folder, box, trees); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(target)
	var got map[string]int
	json.Unmarshal(raw, &got)
	if !reflect.DeepEqual(got, map[string]int{id: 7}) {
		t.Fatal(got)
	}
	if err := e.writeSort(folder, box, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("stale output sort remained", err)
	}
	archiveWrite(t, folder, ".siyuan/sort.json", []byte(`{"stale":99}`))
	os.Remove(filepath.Join(util.DataDir, box, ".siyuan/sort.json"))
	if err := e.writeSort(folder, box, trees); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("missing source kept prior output sort")
	}
}
func TestArchiveExportContextSortBadSourcesAndIOPreserveOutput(t *testing.T) {
	for _, kind := range []string{"corrupt", "null", "source-link", "output-link", "output-directory", "permission"} {
		t.Run(kind, func(t *testing.T) {
			e, box, _ := exportContextFixture(t)
			folder := t.TempDir()
			id := ast.NewNodeID()
			source := archiveWrite(t, util.DataDir, box+"/.siyuan/sort.json", []byte(`{"`+id+`":1}`))
			target := archiveWrite(t, folder, ".siyuan/sort.json", []byte(`{"keep":3}`))
			external := archiveWrite(t, t.TempDir(), "outside", []byte(`{"external":8}`))
			switch kind {
			case "corrupt":
				os.WriteFile(source, []byte("bad"), 0600)
			case "null":
				os.WriteFile(source, []byte("null"), 0600)
			case "source-link":
				os.Remove(source)
				os.Symlink(external, source)
			case "output-link":
				os.Remove(target)
				os.Symlink(external, target)
			case "output-directory":
				os.Remove(target)
				os.Mkdir(target, 0755)
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(filepath.Dir(target), 0500)
				defer os.Chmod(filepath.Dir(target), 0755)
			}
			before, _ := os.ReadFile(target)
			sourceBefore, _ := os.ReadFile(source)
			if err := e.writeSort(folder, box, map[string]*parse.Tree{id: {ID: id}}); err == nil {
				t.Fatal("failed sort reported success", kind)
			}
			after, _ := os.ReadFile(target)
			sourceAfter, _ := os.ReadFile(source)
			if !bytes.Equal(before, after) || !bytes.Equal(sourceBefore, sourceAfter) {
				t.Fatal("failure changed sources/output")
			}
		})
	}
}
func TestArchiveExportContextCanceledChangedClosedAndReadonly(t *testing.T) {
	for _, kind := range []string{"cancel", "old-root", "old-temp", "closed", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			e, box, source := exportContextFixture(t)
			id := util.GetTreeID(source)
			switch kind {
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				e.binding.scope.ctx = ctx
				cancel()
			case "old-root":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "old-temp":
				old := util.TempDir
				util.TempDir = t.TempDir()
				defer func() { util.TempDir = old }()
			case "closed":
				Conf.CloseCloudRepo(context.Background())
			case "readonly":
				Conf.m.Lock()
				Conf.persistenceReadOnly = true
				Conf.m.Unlock()
				defer func() { Conf.m.Lock(); Conf.persistenceReadOnly = false; Conf.m.Unlock() }()
			}
			if _, err := e.single(id); err == nil {
				t.Fatal("single followed invalid owner")
			}
			if _, err := e.notebook(box); err == nil {
				t.Fatal("notebook followed invalid owner")
			}
			if _, err := e.selectedTrees(box, []string{"/" + id + ".docj"}); err == nil {
				t.Fatal("read invalid owner")
			}
			if err := e.writeSort(t.TempDir(), box, nil); err == nil {
				t.Fatal("sort invalid owner")
			}
		})
	}
	if _, err := captureArchiveExport(nil); err == nil {
		t.Fatal("nil context")
	}
}
