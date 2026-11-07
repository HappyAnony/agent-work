//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/util"
)

func TestArchiveExportDocumentExactBytesAndSingleRead(t *testing.T) {
	e, box, source := exportContextFixture(t)
	id := util.GetTreeID(source)
	raw := []byte("{\n  \"ID\":\"" + id + "\",\"Type\":\"NodeDocument\",\"Properties\":{\"id\":\"" + id + "\",\"title\":\"Original\"},\"OpaqueExtension\":{\"keep\": [1,2]},\"Children\": []\n}\n")
	if err := os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	trees, err := e.selectedTrees(box, []string{"/" + id + ".docj"})
	if err != nil {
		t.Fatal(err)
	}
	tree := trees[id]
	if tree.Root.Spec != "1" {
		t.Fatal("detached old-Spec conversion lost")
	}
	folder := t.TempDir()
	for _, action := range []string{"changed", "deleted", "linked"} {
		os.Remove(source)
		switch action {
		case "changed":
			os.WriteFile(source, []byte("new corrupt source"), 0600)
		case "linked":
			os.Symlink(filepath.Join(t.TempDir(), "outside"), source)
		}
		again, err := e.selectedTrees(box, []string{"/" + id + ".docj"})
		if err != nil || again[id] != tree {
			t.Fatal("same export reread source", action, err)
		}
		if err = e.writeDocument(folder, id+".docj", tree); err != nil {
			t.Fatal(action, err)
		}
		got, _ := os.ReadFile(filepath.Join(folder, id+".docj"))
		if !bytes.Equal(raw, got) {
			t.Fatal("format/unknown fields/Spec changed", action)
		}
	}
}
func TestArchiveExportDocumentSelectedAndReferenceShareFirstRead(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	a, b, c := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	exportReferenceDoc(t, e, box, c)
	target := exportReferenceDoc(t, e, box, b, exportReferenceNode(a, true))
	root := exportReferenceDoc(t, e, box, a, exportReferenceNode(b, false))
	file := filepath.Join(util.DataDir, box, b+".docj")
	before, _ := os.ReadFile(file)
	// A later source version introduces another dependency. This export must
	// retain its original selected candidate and the bytes used to build it.
	later := []byte(`{"ID":"` + b + `","Type":"NodeDocument","Children":[{"Type":"NodeTextMark","TextMarkType":"block-ref","TextMarkBlockRefID":"` + c + `","TextMarkBlockRefSubtype":"s"}]}`)
	os.WriteFile(file, later, 0600)
	trees := map[string]*parse.Tree{}
	if err := e.references(root, &[]string{}, trees, map[string]*parse.Tree{}); err != nil {
		t.Fatal(err)
	}
	if len(trees) != 2 || trees[b] != target || trees[c] != nil {
		t.Fatal("selected/reference graphs mixed source revisions")
	}
	folder := t.TempDir()
	if err := e.writeDocument(folder, b+".docj", trees[b]); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(folder, b+".docj"))
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, got) || !bytes.Equal(later, after) {
		t.Fatal("wrong version copied or live source changed")
	}
}
func TestArchiveExportDocumentPathsOverwriteAndIOFailures(t *testing.T) {
	for _, kind := range []string{"nested", "overwrite", "traversal", "absolute", "wrong-name", "linked-file", "linked-parent", "directory", "permission", "linked-root"} {
		t.Run(kind, func(t *testing.T) {
			e, box, source := exportContextFixture(t)
			id := util.GetTreeID(source)
			trees, err := e.selectedTrees(box, []string{"/" + id + ".docj"})
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(source)
			folder := t.TempDir()
			relative := id + ".docj"
			target := filepath.Join(folder, relative)
			outside := filepath.Join(t.TempDir(), id+".docj")
			os.WriteFile(outside, []byte("outside"), 0600)
			switch kind {
			case "nested":
				relative = "parent/child/" + relative
			case "overwrite":
				os.WriteFile(target, []byte("stale output"), 0600)
			case "traversal":
				relative = "../" + relative
			case "absolute":
				relative = "/" + relative
			case "wrong-name":
				relative = ast.NewNodeID() + ".docj"
			case "linked-file":
				os.Symlink(outside, target)
			case "linked-parent":
				os.Symlink(filepath.Dir(outside), filepath.Join(folder, "linked"))
				relative = "linked/" + relative
			case "directory":
				os.Mkdir(target, 0700)
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(folder, 0500)
				defer os.Chmod(folder, 0700)
			case "linked-root":
				link := filepath.Join(t.TempDir(), "root")
				os.Symlink(folder, link)
				folder = link
			}
			err = e.writeDocument(folder, relative, trees[id])
			if kind == "nested" || kind == "overwrite" {
				if err != nil {
					t.Fatal(err)
				}
				got, _ := os.ReadFile(filepath.Join(folder, relative))
				if !bytes.Equal(before, got) {
					t.Fatal("wrong copied bytes")
				}
			} else if err == nil {
				t.Fatal("invalid output accepted", kind)
			}
			after, _ := os.ReadFile(source)
			out, _ := os.ReadFile(outside)
			if !bytes.Equal(before, after) || string(out) != "outside" {
				t.Fatal("source/outside mutated")
			}
		})
	}
}
func TestArchiveExportDocumentIdentityAndUnboundCandidatesReject(t *testing.T) {
	for _, kind := range []string{"nil", "copy", "box", "path", "id", "root-id", "root-box", "root-path"} {
		t.Run(kind, func(t *testing.T) {
			e, box, source := exportContextFixture(t)
			id := util.GetTreeID(source)
			trees, err := e.selectedTrees(box, []string{"/" + id + ".docj"})
			if err != nil {
				t.Fatal(err)
			}
			tree := trees[id]
			switch kind {
			case "nil":
				tree = nil
			case "copy":
				copy := *tree
				tree = &copy
			case "box":
				tree.Box = ast.NewNodeID()
			case "path":
				tree.Path = "/other.docj"
			case "id":
				tree.ID = ast.NewNodeID()
			case "root-id":
				tree.Root.ID = ast.NewNodeID()
			case "root-box":
				tree.Root.Box = ast.NewNodeID()
			case "root-path":
				tree.Root.Path = "/other.docj"
			}
			if err = e.writeDocument(t.TempDir(), id+".docj", tree); err == nil {
				t.Fatal("unbound/changed candidate accepted", kind)
			}
		})
	}
}
func TestArchiveExportDocumentCanceledChangedClosedReadonly(t *testing.T) {
	for _, kind := range []string{"cancel", "old-data", "old-temp", "closed", "readonly"} {
		t.Run(kind, func(t *testing.T) {
			e, box, source := exportContextFixture(t)
			id := util.GetTreeID(source)
			trees, err := e.selectedTrees(box, []string{"/" + id + ".docj"})
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				e.binding.scope.ctx = ctx
				cancel()
			case "old-data":
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
			if err = e.writeDocument(t.TempDir(), id+".docj", trees[id]); err == nil {
				t.Fatal("invalid owner wrote cached bytes", kind)
			}
			if _, err = e.selectedTrees(box, []string{"/" + id + ".docj"}); err == nil {
				t.Fatal("invalid owner reused snapshot", kind)
			}
		})
	}
}
func TestArchiveExportDocumentControllerIsolatesLegacyScratch(t *testing.T) {
	archiveDeckFixture(t)
	e, err := captureArchiveExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	box := e.assetBoxes[0]
	id := ast.NewNodeID()
	exportReferenceDoc(t, e, box, id)
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("keep"), 0600)
	folder := filepath.Join(e.temp, "export", "Reference fixture")
	os.MkdirAll(folder, 0700)
	os.Symlink(outside, filepath.Join(folder, id+".docj"))
	name, zip := ExportSY(id, context.Background())
	if name != id || zip == "" {
		t.Fatal("controller reused legacy scratch", name, zip)
	}
	got, _ := os.ReadFile(outside)
	if string(got) != "keep" {
		t.Fatal("controller followed link")
	}
}

func TestArchiveExportDocumentMovedOrDuplicateIDRejects(t *testing.T) {
	for _, moved := range []bool{false, true} {
		t.Run(map[bool]string{false: "duplicate", true: "moved"}[moved], func(t *testing.T) {
			e, box, source := exportContextFixture(t)
			id := util.GetTreeID(source)
			if _, err := e.selectedTrees(box, []string{"/" + id + ".docj"}); err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(source)
			parent := ast.NewNodeID()
			target := archiveWrite(t, util.DataDir, box+"/"+parent+"/"+id+".docj", raw)
			if moved {
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.selectedTrees(box, []string{"/" + parent + "/" + id + ".docj"}); err == nil {
				t.Fatal("same ID rebound to another path")
			}
			after, _ := os.ReadFile(target)
			if !bytes.Equal(raw, after) {
				t.Fatal("rejected source changed")
			}
		})
	}
}
