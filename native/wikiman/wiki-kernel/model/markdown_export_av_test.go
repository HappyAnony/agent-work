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
	"github.com/88250/lute/render"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/util"
)

func markdownAVFixture(t *testing.T, layout av.LayoutType) (*markdownExportContext, *av.AttributeView, *parse.Tree, string) {
	t.Helper()
	m, box := markdownFixture(t)
	v, block, status := av.NewTableViewWithBlockKey("primary")
	status.ID = "status"
	v.Table.Columns[1].ID = "status"
	number := &av.Key{ID: "number", Name: "Number", Type: av.KeyTypeNumber}
	text := &av.Key{ID: "text", Name: "Hidden | Text", Type: av.KeyTypeText}
	a := &av.AttributeView{ID: ast.NewNodeID(), Spec: 4, Name: "Export AV", Views: []*av.View{v}, ViewID: v.ID, KeyValues: []*av.KeyValues{{Key: block}, {Key: status}, {Key: number}, {Key: text}}}
	for i, name := range []string{"beta | row", "alpha\nrow", "gamma"} {
		row := ast.NewNodeID()
		values := []*av.Value{
			{Type: av.KeyTypeBlock, Block: &av.ValueBlock{ID: row, Content: name}, IsDetached: true},
			{Type: av.KeyTypeSelect, MSelect: []*av.ValueSelect{{Content: "X"}}},
			{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: float64(i + 1), IsNotEmpty: true}},
			{Type: av.KeyTypeText, Text: &av.ValueText{Content: "secret | text"}},
		}
		for j, value := range values {
			value.ID = ast.NewNodeID()
			value.KeyID = a.KeyValues[j].Key.ID
			value.BlockID = row
			value.UpdatedAt = 1
			a.KeyValues[j].Values = append(a.KeyValues[j].Values, value)
		}
	}
	v.Sorts = []*av.ViewSort{{Column: number.ID, Order: av.SortOrderDesc}}
	v.Filters = []*av.ViewFilter{{Column: number.ID, Operator: av.FilterOperatorIsGreater, Value: &av.Value{Type: av.KeyTypeNumber, Number: &av.ValueNumber{Content: 1, IsNotEmpty: true}}}}
	v.PageSize = 1 // Exports all matching records, not only the UI page.
	v.Table.Columns = append(v.Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: number.ID}}, &av.ViewTableColumn{BaseField: &av.BaseField{ID: text.ID, Hidden: true, Wrap: true}})
	v.Table.Columns[0].Wrap = true
	switch layout {
	case av.LayoutTypeGallery:
		v.LayoutType = layout
		v.Gallery = av.NewLayoutGallery()
		for _, c := range v.Table.Columns {
			v.Gallery.CardFields = append(v.Gallery.CardFields, &av.ViewGalleryCardField{BaseField: c.BaseField})
		}
	case av.LayoutTypeKanban:
		v.LayoutType = layout
		v.Kanban = av.NewLayoutKanban()
		for _, c := range v.Table.Columns {
			v.Kanban.Fields = append(v.Kanban.Fields, &av.ViewKanbanField{BaseField: c.BaseField})
		}
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	file := archiveWrite(t, util.DataDir, "storage/av/"+a.ID+".json", raw)
	tree := markdownDoc(t, m, box, "/"+ast.NewNodeID()+".docj", map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeAttributeView", "AttributeViewID": a.ID, "Properties": map[string]string{"custom-av-view": v.ID}})
	return m, a, tree, file
}
func TestMarkdownExportAVOriginalProjectionFilterSortAcrossLayouts(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			m, a, _, file := markdownAVFixture(t, layout)
			before, _ := os.ReadFile(file)
			gotA, table, err := m.exportAttributeTable(a.ID, a.ViewID)
			if err != nil {
				t.Fatal(err)
			}
			if len(table.Rows) != 2 || table.Rows[0].GetValue("number").Number.Content != 3 || table.Rows[1].GetValue("number").Number.Content != 2 {
				t.Fatal("filter/sort/page rules", table)
			}
			if table.GetColumn("text").Hidden != (layout == av.LayoutTypeTable) {
				t.Fatal("original gallery/kanban ID-only visibility changed")
			}
			// Compare meaningful values/columns with the unchanged legacy export pipeline.
			legacy, want := exportRenderAttributeTable(a.ID, a.ViewID, nil)
			if len(want.Rows) != len(table.Rows) || len(want.Columns) != len(table.Columns) {
				t.Fatal("legacy shape differs")
			}
			for i, row := range table.Rows {
				for j, cell := range row.Cells {
					if cell.Value.String(true) != want.Rows[i].Cells[j].Value.String(true) {
						t.Fatal("legacy value differs", i, j)
					}
				}
			}
			if gotA == legacy {
				t.Fatal("shared global AV cache")
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("export rewrote AV")
			}
		})
	}
}
func TestMarkdownExportAVFirstReadCandidateIsolationAndReadonly(t *testing.T) {
	m, a, tree, file := markdownAVFixture(t, av.LayoutTypeTable)
	a.Spec = 3
	oldRaw, _ := json.Marshal(a)
	if err := os.WriteFile(file, oldRaw, 0600); err != nil {
		t.Fatal(err)
	}
	original, err := m.read.readExportView(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), m.read.views[a.ID].raw...)
	// Corrupt disk after admission: the original archive first-read bytes own render.
	if err = os.WriteFile(file, []byte("later invalid file"), 0600); err != nil {
		t.Fatal(err)
	}
	rendered, table, err := m.exportAttributeTable(a.ID, a.ViewID)
	if err != nil {
		t.Fatal(err)
	}
	table.Rows[0].GetValue("number").Number.Content = 999
	rendered.Name = "candidate only"
	again, next, err := m.exportAttributeTable(a.ID, a.ViewID)
	if err != nil || again.Name != "Export AV" || next.Rows[0].GetValue("number").Number.Content != 3 {
		t.Fatal(next, err)
	}
	if original.Spec != 3 || again.Spec != 4 || original.Name != "Export AV" || !bytes.Equal(raw, m.read.views[a.ID].raw) {
		t.Fatal("shared source mutated")
	}
	s, err := m.attributeScope()
	if err != nil {
		t.Fatal(err)
	}
	if err = s.save(again); err == nil {
		t.Fatal("read-only save admitted")
	}
	docFile := filepath.Join(m.read.binding.scope.commit.r.data, tree.Box, tree.ID+".docj")
	before, _ := os.ReadFile(docFile)
	if err = os.WriteFile(docFile, []byte("later invalid doc"), 0600); err != nil {
		t.Fatal(err)
	}
	trees, err := s.trees([]string{tree.ID})
	if err != nil || trees[tree.ID] == nil || trees[tree.ID] == tree {
		t.Fatal(trees, err)
	}
	if len(before) == 0 {
		t.Fatal("empty fixture")
	}
	s.readOnly = false
	if _, err = s.source("storage/av/" + a.ID + ".json"); err == nil {
		t.Fatal("export source admitted writable scope")
	}
	if _, err = s.trees([]string{tree.ID}); err == nil {
		t.Fatal("export trees admitted writable scope")
	}
}
func TestMarkdownExportAVRendererKeepsFormattingAndMissingBlocks(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		t.Run(map[bool]string{false: "all", true: "visible"}[hidden], func(t *testing.T) {
			m, a, tree, file := markdownAVFixture(t, av.LayoutTypeTable)
			before, _ := os.ReadFile(file)
			p := m.referenceRenderer()
			out := exportTree(tree, false, false, hidden, 0, 0, 0, "#", "#", "", "", false, false, false, true, map[string]*parse.Tree{}, p)
			if p.check() != nil || out == nil {
				t.Fatal(p.check())
			}
			md := string(render.NewProtyleExportMdRenderer(out, m.markdownParser().RenderOptions).Render())
			if !strings.Contains(md, "gamma") || !strings.Contains(md, "alpha") || strings.Contains(md, "beta") || strings.Contains(md, "Hidden") == hidden {
				t.Fatal(md)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("source changed")
			}
			// A missing AV still leaves the original dangling block available to callers.
			if _, table, err := m.exportAttributeTable(ast.NewNodeID(), ""); err != nil || table != nil {
				t.Fatal(table, err)
			}
			if a.ID == "" {
				t.Fatal("fixture")
			}
		})
	}
}
func TestMarkdownExportAVFailuresSuppressPartialMarkdown(t *testing.T) {
	for _, kind := range []string{"corrupt", "wrong-id", "linked", "missing-port", "cancel", "owner-closed", "nil-column", "missing-layout"} {
		t.Run(kind, func(t *testing.T) {
			m, a, tree, file := markdownAVFixture(t, av.LayoutTypeTable)
			p := m.referenceRenderer()
			switch kind {
			case "corrupt":
				os.WriteFile(file, []byte("bad json"), 0600)
			case "wrong-id":
				a.ID = ast.NewNodeID()
				raw, _ := json.Marshal(a)
				os.WriteFile(file, raw, 0600)
			case "linked":
				raw, _ := os.ReadFile(file)
				outside := archiveWrite(t, t.TempDir(), "other.json", raw)
				os.Remove(file)
				if err := os.Symlink(outside, file); err != nil {
					t.Fatal(err)
				}
			case "nil-column":
				a.Views[0].Table.Columns = append(a.Views[0].Table.Columns, nil)
				raw, _ := json.Marshal(a)
				os.WriteFile(file, raw, 0600)
			case "missing-layout":
				a.Views[0].Table = nil
				raw, _ := json.Marshal(a)
				os.WriteFile(file, raw, 0600)
			case "missing-port":
				p.attributeTable = nil
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				m.read.binding.scope.ctx = ctx
			case "owner-closed":
				o := m.read.binding.scope.commit.r.owner
				o.repoMu.Lock()
				o.repoClosed = true
				o.repoMu.Unlock()
				t.Cleanup(func() { o.repoMu.Lock(); o.repoClosed = false; o.repoMu.Unlock() })
			}
			result := exportMarkdownContent0(tree.ID, tree, "", false, false, false, ".md", 0, 0, 0, "#", "#", "", "", false, false, nil, true, false, map[string]*parse.Tree{}, p)
			if result != "" || p.check() == nil {
				t.Fatal(kind, result, p.check())
			}
		})
	}
}
func TestMarkdownExportAVRelationRollupTemplateAndCapturedSQL(t *testing.T) {
	m, a, _, file := markdownAVFixture(t, av.LayoutTypeTable)
	a.Views[0].Filters = nil
	a.Views[0].Sorts = nil
	dest, err := cloneAttributeViewPreview(a)
	if err != nil {
		t.Fatal(err)
	}
	dest.ID = ast.NewNodeID()
	dest.KeyValues = dest.KeyValues[:3]
	dest.Views[0].Table.Columns = dest.Views[0].Table.Columns[:3]
	raw, _ := json.Marshal(dest)
	destFile := archiveWrite(t, util.DataDir, "storage/av/"+dest.ID+".json", raw)
	before := append([]byte(nil), raw...)
	rel := &av.Key{ID: "relation", Name: "Relation", Type: av.KeyTypeRelation, Relation: &av.Relation{AvID: dest.ID}}
	rollup := &av.Key{ID: "rollup", Name: "Rollup", Type: av.KeyTypeRollup, Rollup: &av.Rollup{RelationKeyID: rel.ID, KeyID: "number", Calc: &av.RollupCalc{Operator: av.CalcOperatorSum}}}
	id := ast.NewNodeID()
	insertMarkdownRefText(t, m, id, "Captured SQL")
	tpl := &av.Key{ID: "template", Name: "Template", Type: av.KeyTypeTemplate, Template: `.action{ (getBlock "` + id + `").ID }`}
	for _, key := range []*av.Key{rel, rollup, tpl} {
		kv := &av.KeyValues{Key: key}
		for i, primary := range a.KeyValues[0].Values {
			value := &av.Value{ID: ast.NewNodeID(), KeyID: key.ID, BlockID: primary.BlockID, Type: key.Type}
			avrender.FillNilValue(value, key.Type)
			if key == rel {
				value.Relation.BlockIDs = []string{dest.KeyValues[0].Values[i].BlockID}
			}
			kv.Values = append(kv.Values, value)
		}
		a.KeyValues = append(a.KeyValues, kv)
		a.Views[0].Table.Columns = append(a.Views[0].Table.Columns, &av.ViewTableColumn{BaseField: &av.BaseField{ID: key.ID}})
	}
	raw, _ = json.Marshal(a)
	os.WriteFile(file, raw, 0600)
	source := append([]byte(nil), raw...)
	_, table, err := m.exportAttributeTable(a.ID, a.ViewID)
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Rows) != 3 {
		t.Fatal(table)
	}
	for _, row := range table.Rows {
		if row.GetValue(tpl.ID).Template.Content != id || row.GetValue(rel.ID).String(false) == "" || len(row.GetValue(rollup.ID).Rollup.Contents) != 1 || row.GetValue(rollup.ID).Rollup.Contents[0].Number.Content != row.GetValue("number").Number.Content {
			t.Fatal(row)
		}
	}
	after, _ := os.ReadFile(destFile)
	if !bytes.Equal(before, after) {
		t.Fatal("related source changed")
	}
	after, _ = os.ReadFile(file)
	if !bytes.Equal(source, after) {
		t.Fatal("main source changed")
	}
}

func TestMarkdownExportAVZIPSuccessAndFailurePublication(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "table-package", true: "broken-dependency"}[broken], func(t *testing.T) {
			m, _, tree, file := markdownAVFixture(t, av.LayoutTypeTable)
			before, _ := os.ReadFile(file)
			prior := archiveWrite(t, m.read.temp, "export/Prepared Title.md.zip", []byte("prior ZIP"))
			if broken {
				if err := os.WriteFile(file, []byte("broken AV source"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, url := ExportPandocConvertZip([]string{tree.ID}, "", ".md", context.Background())
			if broken {
				if url != "" {
					t.Fatal("failed AV published ZIP", url)
				}
			} else {
				if url == "" {
					t.Fatal("missing ZIP")
				}
				decoded, err := neturl.PathUnescape(filepath.Base(url))
				if err != nil {
					t.Fatal(err)
				}
				members := readOutputZip(t, filepath.Join(m.read.temp, "export", decoded))
				found := false
				for name, raw := range members {
					if strings.HasSuffix(name, ".md") {
						md := string(raw)
						found = strings.Contains(md, "gamma") && strings.Contains(md, "alpha") && !strings.Contains(md, "beta")
					}
				}
				if !found {
					t.Fatal("filtered/sorted AV table not packaged", members)
				}
				after, _ := os.ReadFile(file)
				if !bytes.Equal(before, after) {
					t.Fatal("packaging changed AV source")
				}
			}
			raw, _ := os.ReadFile(prior)
			if string(raw) != "prior ZIP" {
				t.Fatal("prior output overwritten")
			}
			entries, _ := os.ReadDir(m.read.temp)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".wiki-archive-") {
					t.Fatal("scratch retained")
				}
			}
		})
	}
}
