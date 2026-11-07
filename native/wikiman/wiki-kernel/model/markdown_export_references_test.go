//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/util"
)

func insertMarkdownRefText(t *testing.T, m *markdownExportContext, id, text string) {
	t.Helper()
	db := m.read.binding.scope.commit.r.main.DB
	if _, err := db.Exec("INSERT INTO blocks(id,parent_id,root_id,type,content,name,markdown) VALUES(?, '', ?, 'p', ?, '', '')", id, id, text); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE blocks SET hash='',box='',path='',hpath='',alias='',memo='',tag='',fcontent='',length=0,subtype='',ial='',sort=0,created='',updated=''"); err != nil {
		t.Fatal(err)
	}
}
func TestMarkdownExportReferencesSnapshotParserAndAnchor(t *testing.T) {
	m, _ := markdownFixture(t)
	before := m.parserOptions.Markdown
	max := m.anchorMax
	Conf.m.Lock()
	Conf.Editor.BlockRefDynamicAnchorTextMaxLen = max + 99
	Conf.Editor.Markdown.InlineTag = !before.InlineTag
	Conf.m.Unlock()
	if m.anchorMax != max || m.parserOptions.Markdown != before {
		t.Fatal("captured settings changed")
	}
	p := m.referenceRenderer()
	if p.max != max || p.parser() == nil {
		t.Fatal("missing parser capability")
	}
	Conf.m.Lock()
	Conf.Editor = nil
	Conf.m.Unlock()
	if _, err := captureMarkdownExport(context.Background()); err == nil {
		t.Fatal("missing editor admitted")
	}
}
func TestMarkdownExportReferencesDynamicStaticAndMissingText(t *testing.T) {
	m, _ := markdownFixture(t)
	id := ast.NewNodeID()
	insertMarkdownRefText(t, m, id, "__@mark__中文&amp;abcdef__mark@__")
	m.anchorMax = 4
	p := m.referenceRenderer()
	n := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: id}
	got, text := getExportBlockRefLinkText(n, "[", "]", p)
	if got != id || text != "[中文&a...]" || p.check() != nil {
		t.Fatal(got, text, p.check())
	}
	// Container fallback retains the original SQL Index parser's plain-text rules.
	container := ast.NewNodeID()
	insertMarkdownRefText(t, m, container, "")
	if _, err := m.read.binding.scope.commit.r.main.DB.Exec("UPDATE blocks SET type='l', markdown='- first item' WHERE id=?", container); err != nil {
		t.Fatal(err)
	}
	if text, err := p.text(container); err != nil || text != "first item" {
		t.Fatal(text, err)
	}
	n.TextMarkTextContent = "static &amp; text"
	m.anchorMax = 100
	p = m.referenceRenderer()
	_, text = getExportBlockRefLinkText(n, "<", ">", p)
	if text != "<static & text>" || p.check() != nil {
		t.Fatal(text, p.check())
	}
	n.TextMarkTextContent = ""
	n.TextMarkBlockRefID = ast.NewNodeID()
	_, text = getExportBlockRefLinkText(n, "", "", p)
	if text != "block not found" || p.check() != nil {
		t.Fatal(text, p.check())
	}
}
func TestMarkdownExportReferencesSQLFailureIsNotMissingText(t *testing.T) {
	m, _ := markdownFixture(t)
	if err := m.read.binding.scope.commit.r.main.DB.Close(); err != nil {
		t.Fatal(err)
	}
	p := m.referenceRenderer()
	n := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: ast.NewNodeID()}
	getExportBlockRefLinkText(n, "", "", p)
	first := p.check()
	if first == nil {
		t.Fatal("closed SQL became missing text")
	}
	getExportBlockRefLinkText(n, "", "", p)
	if p.check() != first {
		t.Fatal("first failure replaced")
	}
	// Embedded static anchors need no SQL lookup.
	p = m.referenceRenderer()
	n.TextMarkTextContent = "static"
	_, text := getExportBlockRefLinkText(n, "", "", p)
	if text != "static" || p.check() != nil {
		t.Fatal(text, p.check())
	}
}
func TestMarkdownExportReferencesRecursiveLinksReadonlyAndCycle(t *testing.T) {
	m, box := markdownFixture(t)
	a, b := ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+a+".docj", exportReferenceNode(b, true))
	markdownDoc(t, m, box, "/"+b+".docj", exportReferenceNode(a, true))
	files := []string{filepath.Join(util.DataDir, box, a+".docj"), filepath.Join(util.DataDir, box, b+".docj")}
	before := [][]byte{}
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		before = append(before, raw)
	}
	selected, err := m.selectDocuments([]string{a})
	if err != nil {
		t.Fatal(err)
	}
	_, trees, _, err := m.prepare(selected)
	if err != nil {
		t.Fatal(err)
	}
	p := m.referenceRenderer()
	depth := 0
	blockLink2Ref(trees[a], a, trees, &depth, p)
	if p.check() != nil || depth > 4097 {
		t.Fatal(depth, p.check())
	}
	for _, id := range []string{a, b} {
		found := false
		ast.Walk(trees[id].Root, func(n *ast.Node, entering bool) ast.WalkStatus {
			if entering && n.IsTextMarkType("block-ref") {
				found = true
			}
			return ast.WalkContinue
		})
		if !found {
			t.Fatal("link was not converted", id)
		}
	}
	for i, f := range files {
		raw, _ := os.ReadFile(f)
		if !bytes.Equal(before[i], raw) {
			t.Fatal("reference traversal rewrote source")
		}
	}
}
func TestMarkdownExportReferencesLoadAndIdentityErrors(t *testing.T) {
	for _, kind := range []string{"bad-source", "wrong-cache", "closed-registry"} {
		t.Run(kind, func(t *testing.T) {
			m, box := markdownFixture(t)
			id := ast.NewNodeID()
			tree := markdownDoc(t, m, box, "/"+id+".docj")
			p := m.referenceRenderer()
			cache := map[string]*parse.Tree{}
			switch kind {
			case "bad-source":
				os.WriteFile(filepath.Join(util.DataDir, box, id+".docj"), []byte("bad"), 0600)
				fresh, err := captureMarkdownExport(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				p = fresh.referenceRenderer()
			case "wrong-cache":
				tree.Path = "/wrong.docj"
				cache[id] = tree
			case "closed-registry":
				m.read.binding.scope.commit.r.registry.DB.Close()
			}
			if _, err := exportRenderLoad(id, cache, []*exportReferenceRenderer{p}); err == nil || p.check() == nil {
				t.Fatal("bad dependency admitted")
			}
		})
	}
}
func TestMarkdownExportReferencesCancellationAndOwnerSwitch(t *testing.T) {
	for _, kind := range []string{"cancel", "data", "temp"} {
		t.Run(kind, func(t *testing.T) {
			_, _ = markdownFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m, err := captureMarkdownExport(ctx)
			if err != nil {
				t.Fatal(err)
			}
			p := m.referenceRenderer()
			oldData, oldTemp := util.DataDir, util.TempDir
			t.Cleanup(func() { util.DataDir = oldData; util.TempDir = oldTemp })
			switch kind {
			case "cancel":
				cancel()
			case "data":
				util.DataDir = t.TempDir()
			case "temp":
				util.TempDir = t.TempDir()
			}
			if exportRenderLookup(ast.NewNodeID(), []*exportReferenceRenderer{p}) != nil || p.check() == nil {
				t.Fatal("changed owner admitted")
			}
			if kind == "cancel" && !errors.Is(p.check(), context.Canceled) {
				t.Fatal(p.check())
			}
		})
	}
}
func TestMarkdownExportReferencesRenderedModesAndCapturedTruncation(t *testing.T) {
	m, box := markdownFixture(t)
	a, b := ast.NewNodeID(), ast.NewNodeID()
	node := exportReferenceNode(b, false)
	node["TextMarkTextContent"] = ""
	markdownDoc(t, m, box, "/"+a+".docj", map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeParagraph", "Children": []interface{}{node}})
	markdownDoc(t, m, box, "/"+b+".docj")
	insertMarkdownRefText(t, m, b, "Captured dynamic text")
	m.anchorMax = 8
	m.settings.AddTitle = false
	m.settings.MarkdownYFM = false
	Conf.m.Lock()
	Conf.Editor.BlockRefDynamicAnchorTextMaxLen = 1
	Conf.m.Unlock()
	for _, mode := range []int{2, 3, 4} {
		Conf.m.Lock()
		Conf.Editor.BlockRefDynamicAnchorTextMaxLen = 8
		Conf.m.Unlock()
		current, captureErr := captureMarkdownExport(context.Background())
		if captureErr != nil {
			t.Fatal(captureErr)
		}
		current.settings.AddTitle = false
		current.settings.MarkdownYFM = false
		Conf.m.Lock()
		Conf.Editor.BlockRefDynamicAnchorTextMaxLen = 1
		Conf.m.Unlock()
		m = current
		selected, err := m.selectDocuments([]string{a})
		if err != nil {
			t.Fatal(err)
		}
		defs, trees, _, err := m.prepare(selected)
		if err != nil {
			t.Fatal(err)
		}
		_, md, _, err := exportMarkdownContent(a, ".md", mode, defs, false, trees, m)
		if err != nil || !strings.Contains(md, "Captured...") {
			t.Fatal(mode, md, err)
		}
		if mode == 2 && !strings.Contains(md, "siyuan://blocks/"+b) {
			t.Fatal(md)
		}
		if mode == 4 && !strings.Contains(md, "Prepared%20Title.md") && !strings.Contains(md, "Prepared Title.md") {
			t.Fatal(md)
		}
	}
}

func TestMarkdownExportReferencesRenderFailureDoesNotPublishZIP(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj")
	selected, err := m.selectDocuments([]string{id})
	if err != nil {
		t.Fatal(err)
	}
	defs, trees, paths, err := m.prepare(selected)
	if err != nil {
		t.Fatal(err)
	}
	trees[id].Path = "/different.docj"
	old := archiveWrite(t, m.read.temp, "export/Prepared Title.md.zip", []byte("prior"))
	if url := exportPandocConvertZip("Prepared Title", paths, defs, "", "", ".md", trees, m); url != "" {
		t.Fatal("failed reference render published", url)
	}
	raw, _ := os.ReadFile(old)
	if string(raw) != "prior" {
		t.Fatal("prior ZIP changed")
	}
	entries, _ := os.ReadDir(filepath.Join(m.read.temp, "export"))
	if len(entries) != 1 {
		t.Fatal("unexpected artifact", entries)
	}
	scratch, _ := os.ReadDir(m.read.temp)
	for _, entry := range scratch {
		if strings.HasPrefix(entry.Name(), ".wiki-archive-") {
			t.Fatal("scratch leaked")
		}
	}
}
