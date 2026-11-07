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
	"wiki-kernel/util"
)

func exportReferenceDoc(t *testing.T, e *archiveExportContext, box, id string, children ...map[string]interface{}) *parse.Tree {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{"ID": id, "Type": "NodeDocument", "Properties": map[string]string{"id": id, "title": "Reference fixture"}, "Children": children})
	if err != nil {
		t.Fatal(err)
	}
	archiveWrite(t, util.DataDir, box+"/"+id+".docj", raw)
	trees, err := e.selectedTrees(box, []string{"/" + id + ".docj"})
	if err != nil {
		t.Fatal(err)
	}
	tree := trees[id]
	if err = e.binding.scope.commit.r.registry.Upsert(context.Background(), tree); err != nil {
		t.Fatal(err)
	}
	return tree
}
func exportReferenceNode(id string, link bool) map[string]interface{} {
	n := map[string]interface{}{"Type": "NodeTextMark", "TextMarkType": "block-ref", "TextMarkBlockRefID": id, "TextMarkBlockRefSubtype": "s", "TextMarkTextContent": "Reference"}
	if link {
		n = map[string]interface{}{"Type": "NodeTextMark", "TextMarkType": "a", "TextMarkAHref": "siyuan://blocks/" + id, "TextMarkTextContent": "Link"}
	}
	return n
}
func exportReferenceAV(t *testing.T, id, block, recorded string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{"id": id, "spec": 1, "keyValues": []interface{}{map[string]interface{}{"key": map[string]interface{}{"id": "block", "type": "block"}, "values": []interface{}{map[string]interface{}{"id": "value", "type": "block", "blockID": recorded, "block": map[string]interface{}{"id": block}}, map[string]interface{}{"id": "detached", "type": "block", "isDetached": true}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestArchiveExportReferencesOriginalGraphCyclesAndReadonly(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	a, b, c, d, p, avID, recorded := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	exportReferenceDoc(t, e, box, b, exportReferenceNode(a, true), map[string]interface{}{"ID": p, "Type": "NodeParagraph", "Properties": map[string]string{"id": p}})
	exportReferenceDoc(t, e, box, d, exportReferenceNode(p, false))
	exportReferenceDoc(t, e, box, c, map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeAttributeView", "AttributeViewID": avID})
	root := exportReferenceDoc(t, e, box, a, exportReferenceNode(p, false), exportReferenceNode(c, true), exportReferenceNode(ast.NewNodeID(), false))
	raw := exportReferenceAV(t, avID, d, recorded)
	file := archiveWrite(t, util.DataDir, "storage/av/"+avID+".json", raw)
	before := map[string][]byte{}
	for _, id := range []string{a, b, c, d} {
		name := filepath.Join(util.DataDir, box, id+".docj")
		before[name], _ = os.ReadFile(name)
	}
	trees, cache := map[string]*parse.Tree{}, map[string]*parse.Tree{}
	ids := []string{}
	if err := e.references(root, &ids, trees, cache); err != nil {
		t.Fatal(err)
	}
	if len(trees) != 4 || !reflect.DeepEqual(ids, []string{p, a, c, recorded}) {
		t.Fatal("original graph/order/dedup rules", len(trees), ids)
	}
	for name, want := range before {
		got, _ := os.ReadFile(name)
		if !bytes.Equal(want, got) {
			t.Fatal("reference reader changed source", name)
		}
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(raw, after) {
		t.Fatal("reference AV was upgraded")
	}
	// A per-export cache retains the first detached candidate even if disk changes.
	os.WriteFile(filepath.Join(util.DataDir, box, b+".docj"), []byte("changed after capture"), 0600)
	if err := e.references(root, &[]string{}, map[string]*parse.Tree{}, cache); err != nil {
		t.Fatal("cache not reused", err)
	}
}
func TestArchiveExportReferencesDanglingAndEmptyAVPreserved(t *testing.T) {
	for _, kind := range []string{"missing-index", "missing-av", "empty-av"} {
		t.Run(kind, func(t *testing.T) {
			e, box, _ := exportContextFixture(t)
			id, avID := ast.NewNodeID(), ast.NewNodeID()
			node := exportReferenceNode(ast.NewNodeID(), false)
			if kind != "missing-index" {
				node = map[string]interface{}{"Type": "NodeAttributeView", "AttributeViewID": avID}
			}
			if kind == "empty-av" {
				archiveWrite(t, util.DataDir, "storage/av/"+avID+".json", []byte(`{"id":"`+avID+`","keyValues":[]}`))
			}
			root := exportReferenceDoc(t, e, box, id, node)
			trees := map[string]*parse.Tree{}
			if err := e.references(root, &[]string{}, trees, map[string]*parse.Tree{}); err != nil || len(trees) != 1 {
				t.Fatal(kind, trees, err)
			}
		})
	}
}
func TestArchiveExportReferencesDocumentFailuresAndCacheIdentity(t *testing.T) {
	for _, kind := range []string{"bad-doc", "missing-doc", "linked-doc", "permission-doc", "registry", "closed-box", "bad-cache"} {
		t.Run(kind, func(t *testing.T) {
			e, box, _ := exportContextFixture(t)
			a, b := ast.NewNodeID(), ast.NewNodeID()
			target := exportReferenceDoc(t, e, box, b)
			root := exportReferenceDoc(t, e, box, a, exportReferenceNode(b, false))
			// Setup used an export loader to seed the registry. A new export must
			// observe these first-read failures rather than reuse setup's snapshots.
			e, err := captureArchiveExport(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(util.DataDir, box, b+".docj")
			cache := map[string]*parse.Tree{}
			switch kind {
			case "bad-doc":
				os.WriteFile(filename, []byte("bad"), 0600)
			case "missing-doc":
				os.Remove(filename)
			case "linked-doc":
				os.Remove(filename)
				os.Symlink(filepath.Join(t.TempDir(), "outside"), filename)
			case "permission-doc":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(filename, 0000)
				defer os.Chmod(filename, 0600)
			case "registry":
				_, err := e.binding.scope.commit.r.registry.DB.Exec("ALTER TABLE blocktrees RENAME TO hidden_reference_probe")
				if err != nil {
					t.Fatal(err)
				}
				defer e.binding.scope.commit.r.registry.DB.Exec("ALTER TABLE hidden_reference_probe RENAME TO blocktrees")
			case "closed-box":
				delete(e.boxes, box)
			case "bad-cache":
				copy := *target
				copy.Box = ast.NewNodeID()
				cache[b] = &copy
			}
			if err := e.references(root, &[]string{}, map[string]*parse.Tree{}, cache); err == nil {
				t.Fatal("dependency failure swallowed", kind)
			}
		})
	}
}
func TestArchiveExportReferencesAVFailures(t *testing.T) {
	for _, kind := range []string{"json", "null", "wrong-id", "null-key", "null-value", "nil-block", "linked", "permission", "invalid-id"} {
		t.Run(kind, func(t *testing.T) {
			e, box, _ := exportContextFixture(t)
			id := ast.NewNodeID()
			root := exportReferenceDoc(t, e, box, ast.NewNodeID(), map[string]interface{}{"Type": "NodeAttributeView", "AttributeViewID": id})
			raw := exportReferenceAV(t, id, ast.NewNodeID(), ast.NewNodeID())
			switch kind {
			case "json":
				raw = []byte("bad")
			case "null":
				raw = []byte("null")
			case "wrong-id":
				raw = []byte(`{"id":"` + ast.NewNodeID() + `"}`)
			case "null-key":
				raw = []byte(`{"id":"` + id + `","keyValues":[null]}`)
			case "null-value":
				raw = []byte(`{"id":"` + id + `","keyValues":[{"key":{"id":"block","type":"block"},"values":[null]}]}`)
			case "nil-block":
				raw = []byte(`{"id":"` + id + `","keyValues":[{"key":{"id":"block","type":"block"},"values":[{"isDetached":false}]}]}`)
			case "invalid-id":
				root.Root.FirstChild.AttributeViewID = "../outside"
			}
			file := archiveWrite(t, util.DataDir, "storage/av/"+id+".json", raw)
			if kind == "linked" {
				os.Remove(file)
				os.Symlink(filepath.Join(t.TempDir(), "outside"), file)
			}
			if kind == "permission" {
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(file, 0000)
				defer os.Chmod(file, 0600)
			}
			if err := e.references(root, &[]string{}, map[string]*parse.Tree{}, map[string]*parse.Tree{}); err == nil {
				t.Fatal("bad AV dependency swallowed", kind)
			}
		})
	}
}
func TestArchiveExportReferencesCanceledChangedClosedAndMidTraversal(t *testing.T) {
	for _, kind := range []string{"canceled", "data-changed", "temp-changed", "closed"} {
		t.Run(kind, func(t *testing.T) {
			e, box, _ := exportContextFixture(t)
			root := exportReferenceDoc(t, e, box, ast.NewNodeID())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.binding.scope.ctx = ctx
			switch kind {
			case "canceled":
				cancel()
			case "data-changed":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "temp-changed":
				old := util.TempDir
				util.TempDir = t.TempDir()
				defer func() { util.TempDir = old }()
			case "closed":
				Conf.CloseCloudRepo(context.Background())
			}
			if err := e.references(root, &[]string{}, map[string]*parse.Tree{}, map[string]*parse.Tree{}); err == nil {
				t.Fatal("invalid owner accepted", kind)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := &parse.Tree{ID: ast.NewNodeID(), Root: &ast.Node{Type: ast.NodeDocument}}
	root.Root.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: ast.NewNodeID()})
	err := walkExportReferences(root, &[]string{}, map[string]*parse.Tree{}, exportReferenceServices{guard: ctx.Err, resolve: func(string) (*parse.Tree, error) {
		cancel()
		return &parse.Tree{ID: ast.NewNodeID(), Root: &ast.Node{Type: ast.NodeDocument}}, nil
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation during traversal swallowed", err)
	}
}
func TestArchiveExportReferencesControllerRejectsBadDependency(t *testing.T) {
	e, box, _ := exportContextFixture(t)
	a, b := ast.NewNodeID(), ast.NewNodeID()
	exportReferenceDoc(t, e, box, b)
	exportReferenceDoc(t, e, box, a, exportReferenceNode(b, true))
	os.WriteFile(filepath.Join(util.DataDir, box, b+".docj"), []byte("bad dependency"), 0600)
	name, zip := ExportSY(a, context.Background())
	if name != a || zip != "" {
		t.Fatal("controller reported partial archive success", name, zip)
	}
}
