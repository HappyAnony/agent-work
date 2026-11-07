//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/treenode"
)

func TestHTMLPreviewFocusedHeadingAndRepeatedSourceIsolation(t *testing.T) {
	m, box, ids := focusFixture(t)
	file := filepath.Join(m.read.binding.scope.commit.r.data, box, ids[0]+".docj")
	before, _ := os.ReadFile(file)
	for _, title := range []bool{false, true} {
		m.settings.AddTitle = title
		html, err := m.preview(ids[2], false)
		level := "<h1"
		if title {
			level = "<h2"
		}
		if err != nil || !strings.Contains(html, level) || !strings.Contains(html, "Focused body") || !strings.Contains(html, "Nested body") || strings.Contains(html, "Before") || strings.Contains(html, "Next peer") {
			t.Fatal(html, err)
		}
	}
	html, err := m.preview(ids[0], false)
	if err != nil || !strings.Contains(html, "Before") || !strings.Contains(html, "Next peer") {
		t.Fatal(html, err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
	if m.read.output != nil {
		t.Fatal("preview opened publication")
	}
}
func TestHTMLPreviewDerivedThemeAndCapturedPreferences(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj", map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeParagraph", "Properties": map[string]string{"style": "color: var(--ink);"}, "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": "Styled preview"}}})
	m.themeRoot = t.TempDir()
	m.themeName = "custom"
	m.themeBound = true
	m.settings.AddTitle = false
	theme := archiveWrite(t, m.themeRoot, "custom/theme.css", []byte(`:root { --ink: #123456; }`))
	file := filepath.Join(m.read.binding.scope.commit.r.data, box, id+".docj")
	before, _ := os.ReadFile(file)
	plain, err := m.preview(id, false)
	if err != nil || !strings.Contains(plain, "var(--ink)") {
		t.Fatal(plain, err)
	}
	Conf.m.Lock()
	Conf.Export = nil
	Conf.Appearance = nil
	Conf.m.Unlock()
	html, err := m.preview(id, true)
	if err != nil || !strings.Contains(html, "#123456") || strings.Contains(html, "var(--ink)") {
		t.Fatal(html, err)
	}
	os.WriteFile(theme, []byte(`:root { --ink: red; }`), 0600)
	again, err := m.preview(id, true)
	if err != nil || !strings.Contains(again, "#123456") {
		t.Fatal(again, err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("source style rewritten")
	}
}
func TestHTMLPreviewAVOriginalThreeLayoutProjection(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			m, _, tree, file := markdownAVFixture(t, layout)
			before, _ := os.ReadFile(file)
			html, err := m.preview(tree.ID, false)
			if err != nil || !strings.Contains(html, "<table") || !strings.Contains(html, "gamma") || !strings.Contains(html, "alpha") || strings.Contains(html, "beta") || strings.Index(html, "gamma") > strings.Index(html, "alpha") {
				t.Fatal(html, err)
			}
			if strings.Contains(html, "secret | text") != (layout != av.LayoutTypeTable) {
				t.Fatal("original hidden-column projection changed", html)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("AV source changed")
			}
		})
	}
}
func TestHTMLPreviewReferenceFootnotesAndOwnedQuery(t *testing.T) {
	m, box := markdownFixture(t)
	m.settings.BlockRefMode = 4
	m.settings.AddTitle = false
	target, paragraph, source := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+target+".docj", embedTextNode(paragraph, "Referenced HTML body"))
	embedSQLRow(t, m, paragraph, "p")
	markdownDoc(t, m, box, "/"+source+".docj", footParagraph(ast.NewNodeID(), footRef(paragraph, "Static preview reference")), embedQueryNode(ast.NewNodeID(), "SELECT * FROM blocks WHERE id='"+paragraph+"'"))
	html, err := m.preview(source, false)
	if err != nil || !strings.Contains(html, "Referenced HTML body") || !strings.Contains(html, "Static preview reference") || !strings.Contains(html, "footnotes") {
		t.Fatal(html, err)
	}
}
func TestHTMLPreviewFailuresReturnNoPartialHTML(t *testing.T) {
	for _, kind := range []string{"query", "theme-link", "canceled", "closed", "missing"} {
		t.Run(kind, func(t *testing.T) {
			m, box := markdownFixture(t)
			id := ast.NewNodeID()
			node := embedTextNode(ast.NewNodeID(), "preview prefix")
			if kind == "query" {
				node = embedQueryNode(ast.NewNodeID(), "SELECT * FROM invalid_preview_table")
			}
			markdownDoc(t, m, box, "/"+id+".docj", node)
			fill := false
			switch kind {
			case "theme-link":
				m.themeRoot = t.TempDir()
				m.themeBound = true
				m.themeName = "custom"
				outside := archiveWrite(t, t.TempDir(), "theme.css", []byte(`:root { --x: red; }`))
				os.MkdirAll(filepath.Join(m.themeRoot, "custom"), 0700)
				if err := os.Symlink(outside, filepath.Join(m.themeRoot, "custom/theme.css")); err != nil {
					t.Fatal(err)
				}
				fill = true
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				m.read.binding.scope.ctx = ctx
			case "closed":
				owner := m.read.binding.scope.commit.r.owner
				owner.repoMu.Lock()
				owner.repoClosed = true
				owner.repoMu.Unlock()
				t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
			case "missing":
				id = ast.NewNodeID()
			}
			if html, err := m.preview(id, fill); err == nil || html != "" {
				t.Fatal(html, err)
			}
		})
	}
	if html, err := PreviewContext(nil, ast.NewNodeID(), false); err == nil || html != "" {
		t.Fatal(html, err)
	}
}
func TestHTMLPreviewOriginalFootnoteSuffixAndSuperblockIALRules(t *testing.T) {
	m, _ := markdownFixture(t)
	engine := m.referenceRenderer().parser()
	engine.SetFootnotes(true)
	tree := parse.Parse("", []byte("Text[^1] after\n\n[^1]: Note body\n"), engine.ParseOptions)
	var ref *ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter && n.Type == ast.NodeFootnotesRef {
			ref = n
		}
		return ast.WalkContinue
	})
	if ref == nil {
		t.Fatal("footnote fixture not parsed")
	}
	ref.InsertAfter(&ast.Node{Type: ast.NodeText, Tokens: []byte("(suffix)")})
	html, err := renderHTMLPreview(tree, &treenode.BlockTree{Type: "d"}, engine, false, nil, func() error { return nil })
	if err != nil || !strings.Contains(html, "Note body") || ref.Next == nil || string(ref.Next.Tokens) != editor.Zwsp {
		t.Fatal(html, err)
	}
	engine = m.referenceRenderer().parser()
	root := &ast.Node{Type: ast.NodeDocument}
	super := &ast.Node{Type: ast.NodeSuperBlock, KramdownIAL: [][]string{{"id", ast.NewNodeID()}}}
	root.AppendChild(super)
	paragraph := &ast.Node{Type: ast.NodeParagraph}
	paragraph.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("Superblock body")})
	super.AppendChild(paragraph)
	tree = &parse.Tree{Root: root}
	_, err = renderHTMLPreview(tree, &treenode.BlockTree{Type: "d"}, engine, false, nil, func() error { return nil })
	if err != nil || super.Next != nil && super.Next.Type == ast.NodeKramdownBlockIAL {
		t.Fatal("superblock IAL survived", err)
	}
	calls := 0
	engine = m.referenceRenderer().parser()
	tree = parse.Parse("", []byte("guarded preview"), engine.ParseOptions)
	html, err = renderHTMLPreview(tree, &treenode.BlockTree{Type: "d"}, engine, false, nil, func() error {
		calls++
		if calls > 2 {
			return context.Canceled
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) || html != "" {
		t.Fatal(html, err)
	}
	if html, err = renderHTMLPreview(nil, nil, nil, false, nil, nil); err == nil || html != "" {
		t.Fatal(html, err)
	}
}
