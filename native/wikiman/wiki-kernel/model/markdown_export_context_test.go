//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func markdownFixture(t *testing.T) (*markdownExportContext, string) {
	t.Helper()
	_, box, _ := exportContextFixture(t)
	Conf.m.Lock()
	Conf.Export = conf.NewExport()
	Conf.m.Unlock()
	m, err := captureMarkdownExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return m, box
}
func markdownDoc(t *testing.T, m *markdownExportContext, box, p string, children ...map[string]interface{}) *parse.Tree {
	t.Helper()
	id := util.GetTreeID(p)
	if len(children) == 0 {
		children = []map[string]interface{}{{"ID": ast.NewNodeID(), "Type": "NodeParagraph", "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": "Prepared content"}}}}
	}
	raw, err := json.Marshal(map[string]interface{}{"ID": id, "Type": "NodeDocument", "Properties": map[string]string{"id": id, "title": "Prepared Title"}, "Opaque": map[string]string{"keep": "native"}, "Children": children})
	if err != nil {
		t.Fatal(err)
	}
	archiveWrite(t, util.DataDir, box+p, raw)
	trees, err := m.read.selectedTrees(box, []string{p})
	if err != nil {
		t.Fatal(err)
	}
	tree := trees[id]
	if err = m.read.binding.scope.commit.r.registry.Upsert(context.Background(), tree); err != nil {
		t.Fatal(err)
	}
	return tree
}
func TestMarkdownExportContextCapturesDetachedPreferences(t *testing.T) {
	m, _ := markdownFixture(t)
	before := m.settings
	Conf.m.Lock()
	Conf.Export.BlockRefMode = 2
	Conf.Export.MarkdownYFM = !before.MarkdownYFM
	Conf.Export.TagOpenMarker = "later"
	Conf.m.Unlock()
	if m.settings != before {
		t.Fatal("snapshot followed current settings")
	}
	Conf.m.Lock()
	Conf.Export = nil
	Conf.m.Unlock()
	if _, err := captureMarkdownExport(context.Background()); err == nil {
		t.Fatal("missing export preferences accepted")
	}
	if _, err := captureMarkdownExport(nil); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := captureMarkdownExport(ctx); err == nil {
		t.Fatal("canceled capture accepted")
	}
}
func TestMarkdownExportContextOriginalGraphDescendantsAndReadonlySources(t *testing.T) {
	m, box := markdownFixture(t)
	a, b, c, d, avID, recorded := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+a+".docj", exportReferenceNode(c, false), resourceAVNode(avID))
	markdownDoc(t, m, box, "/"+a+"/"+b+".docj")
	markdownDoc(t, m, box, "/"+c+".docj", exportReferenceNode(a, true))
	markdownDoc(t, m, box, "/"+d+".docj")
	avRaw := exportReferenceAV(t, avID, d, recorded)
	avFile := archiveWrite(t, util.DataDir, "storage/av/"+avID+".json", avRaw)
	before := map[string][]byte{}
	for _, p := range []string{"/" + a + ".docj", "/" + a + "/" + b + ".docj", "/" + c + ".docj", "/" + d + ".docj"} {
		name := filepath.Join(util.DataDir, box, p)
		before[name], _ = os.ReadFile(name)
	}
	selections, err := m.selectDocuments([]string{a})
	if err != nil || len(selections[0].paths) != 2 {
		t.Fatal(selections, err)
	}
	defs, trees, paths, err := m.prepare(selections)
	if err != nil || len(trees) != 4 || len(paths) != 4 {
		t.Fatal(trees, paths, err)
	}
	for _, id := range []string{a, c, recorded} {
		found := false
		for _, def := range defs {
			found = found || def == id
		}
		if !found {
			t.Fatal("original reference IDs lost", defs, id)
		}
	}
	for name, raw := range before {
		after, _ := os.ReadFile(name)
		if !bytes.Equal(raw, after) {
			t.Fatal("source repaired", name)
		}
	}
	afterAV, _ := os.ReadFile(avFile)
	if !bytes.Equal(avRaw, afterAV) {
		t.Fatal("AV source upgraded")
	}
	for _, tree := range trees {
		if tree.Root.Spec != "1" {
			t.Fatal("candidate-only old Spec conversion lost")
		}
	}
}
func TestMarkdownExportContextSelectionUsesEachNotebook(t *testing.T) {
	m, box := markdownFixture(t)
	other := ast.NewNodeID()
	m.read.boxes[other] = Box{ID: other, Name: "Other"}
	a, b, c, d := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+a+".docj")
	markdownDoc(t, m, box, "/"+a+"/"+b+".docj")
	markdownDoc(t, m, other, "/"+c+".docj")
	markdownDoc(t, m, other, "/"+c+"/"+d+".docj")
	selections, err := m.selectDocuments([]string{a, c})
	if err != nil || len(selections) != 2 || len(selections[1].paths) != 2 {
		t.Fatal("wrong notebook used for descendants", selections, err)
	}
	_, trees, _, err := m.prepare(selections)
	if err != nil || len(trees) != 4 || trees[b].Box != box || trees[d].Box != other {
		t.Fatal(trees, err)
	}
}
func TestMarkdownExportContextFirstReadFailuresReturned(t *testing.T) {
	for _, kind := range []string{"bad", "missing", "link", "permission", "registry", "closed"} {
		t.Run(kind, func(t *testing.T) {
			m, box := markdownFixture(t)
			id := ast.NewNodeID()
			markdownDoc(t, m, box, "/"+id+".docj")
			m, err := captureMarkdownExport(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(util.DataDir, box, id+".docj")
			switch kind {
			case "bad":
				os.WriteFile(file, []byte("bad"), 0600)
			case "missing":
				os.Remove(file)
			case "link":
				os.Remove(file)
				os.Symlink(filepath.Join(t.TempDir(), "outside"), file)
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses permissions")
				}
				os.Chmod(file, 0000)
				defer os.Chmod(file, 0600)
			case "registry":
				_, err = m.read.binding.scope.commit.r.registry.DB.Exec("ALTER TABLE blocktrees RENAME TO markdown_hidden_tree")
				if err != nil {
					t.Fatal(err)
				}
				defer m.read.binding.scope.commit.r.registry.DB.Exec("ALTER TABLE markdown_hidden_tree RENAME TO blocktrees")
			case "closed":
				Conf.CloseCloudRepo(context.Background())
			}
			selected, err := m.selectDocuments([]string{id})
			if err == nil {
				_, _, _, err = m.prepare(selected)
			}
			if err == nil {
				t.Fatal("selection/load failure swallowed", kind)
			}
		})
	}
}
func TestMarkdownExportContextCapturedCountsAndGuardFailures(t *testing.T) {
	for _, kind := range []string{"sql", "cancel", "data-switch", "temp-switch"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := markdownFixture(t)
			switch kind {
			case "sql":
				m.read.binding.scope.commit.r.main.DB.Close()
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				m.read.binding.scope.ctx = ctx
				cancel()
			case "data-switch":
				old := util.DataDir
				util.DataDir = t.TempDir()
				defer func() { util.DataDir = old }()
			case "temp-switch":
				old := util.TempDir
				util.TempDir = t.TempDir()
				defer func() { util.TempDir = old }()
			}
			if _, err := m.rootChildrenRefCounts(ast.NewNodeID()); err == nil {
				t.Fatal("captured query/guard error ignored", kind)
			}
		})
	}
}
func TestMarkdownExportContextTopLevelRenderUsesCapturedOptions(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj")
	file := filepath.Join(util.DataDir, box, id+".docj")
	before, _ := os.ReadFile(file)
	m.settings.AddTitle = true
	m.settings.MarkdownYFM = true
	selections, err := m.selectDocuments([]string{id})
	if err != nil {
		t.Fatal(err)
	}
	defs, trees, _, err := m.prepare(selections)
	if err != nil {
		t.Fatal(err)
	}
	Conf.m.Lock()
	Conf.Export.AddTitle = false
	Conf.Export.MarkdownYFM = false
	Conf.m.Unlock()
	_, text, _, err := exportMarkdownContent(id, ".md", m.settings.BlockRefMode, defs, false, trees, m)
	if err != nil || !strings.HasPrefix(text, "---\n") || !strings.Contains(text, "# Prepared Title") {
		t.Fatal("captured title/YFM options lost", text, err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("plain document source changed")
	}
}
func TestMarkdownExportContextBadPreparationDoesNotTouchOldOutput(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj")
	os.WriteFile(filepath.Join(util.DataDir, box, id+".docj"), []byte("bad"), 0600)
	old := archiveWrite(t, m.read.temp, "export/Prepared Title.md/keep", []byte("prior output"))
	name, zip := ExportPandocConvertZip([]string{id}, "", ".md", context.Background())
	if name != "" || zip != "" {
		t.Fatal("bad preparation reported success", name, zip)
	}
	raw, _ := os.ReadFile(old)
	if string(raw) != "prior output" {
		t.Fatal("old output destroyed before preparation completed")
	}
	if name, zip = ExportPandocConvertZip(nil, "", ".md"); name != "" || zip != "" {
		t.Fatal("empty selection accepted")
	}
}
