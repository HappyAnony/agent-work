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
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/util"
)

func TestMarkdownExportTitleMatchesLegacyContentAndIAL(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	tree := markdownDoc(t, m, box, "/"+id+".docj")
	tree.Root.SetIALAttr("title", "中文 &amp; <Title>\u200b")
	tree.Root.SetIALAttr("custom-key", "kept &amp; escaped")
	tree.Root.SetIALAttr("name", "Name &amp; alias")
	before := make([][]string, len(tree.Root.KramdownIAL))
	for i, kv := range tree.Root.KramdownIAL {
		before[i] = append([]string(nil), kv...)
	}
	got, err := m.exportDocumentTitle(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, tree.Root.KramdownIAL) {
		t.Fatal("IAL changed during projection")
	}
	want, err := getBlock(id, tree)
	if err != nil || got.Content != want.Content || got.FContent != want.FContent || !reflect.DeepEqual(got.IAL, want.IAL) || got.Name != want.Name {
		t.Fatal(got, want, err)
	}
	got.IAL["custom-key"] = "modified result"
	if tree.Root.IALAttr("custom-key") != "kept &amp; escaped" {
		t.Fatal("result aliases tree")
	}
}
func TestMarkdownExportTitleRootOnlyDoesNotNeedSQLOrRegistry(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	tree := markdownDoc(t, m, box, "/"+id+".docj")
	m.read.binding.scope.commit.r.main.DB.Close()
	m.read.binding.scope.commit.r.registry.DB.Close()
	got, err := m.exportDocumentTitle(tree)
	if err != nil || got.Content != "Prepared Title" {
		t.Fatal(got, err)
	}
}
func TestMarkdownExportTitleCapturedSearchAndPlainParser(t *testing.T) {
	m, _ := markdownFixture(t)
	before := m.parserOptions.Markdown
	caseBefore := m.searchCaseSensitive
	Conf.m.Lock()
	Conf.Editor.Markdown.InlineAsterisk = !before.InlineAsterisk
	Conf.Search.CaseSensitive = !caseBefore
	Conf.m.Unlock()
	if m.markdownParser().ParseOptions.InlineAsterisk != before.InlineAsterisk || m.searchCaseSensitive != caseBefore {
		t.Fatal("parser/search snapshot followed global")
	}
	p := m.referenceRenderer()
	if exportRenderMarkdownParser([]*exportReferenceRenderer{p}).ParseOptions.InlineAsterisk != before.InlineAsterisk {
		t.Fatal("widget parser followed global")
	}
}
func TestMarkdownExportTitleRenderedEscapingLengthAndSourceReadonly(t *testing.T) {
	for _, title := range []string{"中文 &amp; <Title>\u200b", strings.Repeat("长", 5200)} {
		t.Run(map[bool]string{true: "long", false: "escaped"}[len(title) > 1000], func(t *testing.T) {
			m, box := markdownFixture(t)
			id := ast.NewNodeID()
			tree := markdownDoc(t, m, box, "/"+id+".docj")
			tree.Root.SetIALAttr("title", title)
			tree.Root.SetIALAttr("custom-title", "kept")
			file := filepath.Join(util.DataDir, box, id+".docj")
			before, _ := os.ReadFile(file)
			want, err := m.exportDocumentTitle(tree)
			if err != nil {
				t.Fatal(err)
			}
			m.settings.AddTitle = true
			m.settings.MarkdownYFM = false
			_, md, _, err := exportMarkdownContent(id, ".md", 2, nil, false, map[string]*parse.Tree{id: tree}, m)
			if err != nil || !strings.HasPrefix(md, "# ") {
				t.Fatal(md[:min(len(md), 100)], err)
			}
			if len(title) > 1000 {
				if len([]rune(want.Content)) != 5123 || !strings.HasSuffix(want.Content, "...") {
					t.Fatal(len([]rune(want.Content)))
				}
			} else if !strings.Contains(md, "中文") || !strings.Contains(md, "Title") {
				t.Fatal(md)
			}
			if rootHeading := tree.Root.FirstChild; rootHeading.Type != ast.NodeHeading || rootHeading.IALAttr("custom-title") != "kept" {
				t.Fatal("heading attributes lost")
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("title wrote source")
			}
		})
	}
}
func TestMarkdownExportTitleFailureIdentityGuardAndNoZIP(t *testing.T) {
	for _, kind := range []string{"unbound", "identity", "ial", "cancel", "temp"} {
		t.Run(kind, func(t *testing.T) {
			m, box := markdownFixture(t)
			id := ast.NewNodeID()
			tree := markdownDoc(t, m, box, "/"+id+".docj")
			oldTemp := util.TempDir
			t.Cleanup(func() { util.TempDir = oldTemp })
			switch kind {
			case "unbound":
				delete(m.read.documents, id)
			case "identity":
				tree.Root.ID = ast.NewNodeID()
			case "ial":
				tree.Root.KramdownIAL = append(tree.Root.KramdownIAL, []string{"invalid"})
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				m.read.binding.scope.ctx = ctx
				cancel()
			case "temp":
				util.TempDir = t.TempDir()
			}
			if _, err := m.exportDocumentTitle(tree); err == nil {
				t.Fatal("invalid title projection accepted")
			}
		})
	}
	// Malformed IAL passes selection identity checks but must not publish a ZIP.
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	tree := markdownDoc(t, m, box, "/"+id+".docj")
	old := archiveWrite(t, m.read.temp, "export/Prepared Title.md.zip", []byte("prior"))
	tree.Root.KramdownIAL = append(tree.Root.KramdownIAL, []string{"invalid"})
	m.settings.AddTitle = true
	p := m.referenceRenderer()
	if exportRenderDocumentTitle(id, tree, []*exportReferenceRenderer{p}) != nil || p.check() == nil {
		t.Fatal("bad IAL not propagated")
	}
	if url := exportPandocConvertZip("Prepared Title", []string{"/" + id + ".docj"}, nil, "", "", ".md", map[string]*parse.Tree{id: tree}, m); url != "" {
		t.Fatal("bad title published", url)
	}
	raw, _ := os.ReadFile(old)
	if string(raw) != "prior" {
		t.Fatal("old output changed")
	}
	entries, _ := os.ReadDir(m.read.temp)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".wiki-archive-") {
			t.Fatal("scratch remains")
		}
	}
	exports, _ := os.ReadDir(filepath.Join(m.read.temp, "export"))
	if len(exports) != 1 {
		t.Fatal(exports)
	}

}
func TestMarkdownExportTitleWidgetMarkdownKeepsOriginalRendering(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	widget := map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeWidget", "Properties": map[string]string{"data-export-md": "---\nname: widget\n---\n\n**Widget strong**\n\n`a&amp;b`"}}
	tree := markdownDoc(t, m, box, "/"+id+".docj", widget)
	m.settings.AddTitle = false
	m.settings.MarkdownYFM = false
	_, md, _, err := exportMarkdownContent(id, ".md", 2, nil, false, map[string]*parse.Tree{id: tree}, m)
	if err != nil || !strings.Contains(md, "Widget strong") || !strings.Contains(md, "a&b") || !strings.Contains(md, "name: widget") {
		t.Fatal(md, err)
	}
	if len(tree.Root.ChildrenByType(ast.NodeWidget)) != 0 {
		t.Fatal("widget was not replaced")
	}
}
