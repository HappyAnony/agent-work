//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/util"
)

func footRef(id, text string) map[string]interface{} {
	n := exportReferenceNode(id, false)
	n["TextMarkTextContent"] = text
	return n
}
func footParagraph(id string, children ...interface{}) map[string]interface{} {
	return map[string]interface{}{"ID": id, "Type": "NodeParagraph", "Children": children}
}
func footText(t *testing.T, p *exportReferenceRenderer, n *ast.Node) string {
	t.Helper()
	if n == nil {
		return ""
	}
	text, err := lute.FormatNodeSync(n, p.parser().ParseOptions, p.parser().RenderOptions)
	if err != nil {
		t.Fatal(err)
	}
	return text
}
func footPrepared(t *testing.T, m *markdownExportContext, id string) ([]string, map[string]*parse.Tree) {
	t.Helper()
	selected, err := m.selectDocuments([]string{id})
	if err != nil {
		t.Fatal(err)
	}
	defs, trees, _, err := m.prepare(selected)
	if err != nil {
		t.Fatal(err)
	}
	return defs, trees
}
func TestMarkdownExportFootnotesCollectionOrderDedupAndSnapshot(t *testing.T) {
	m, box := markdownFixture(t)
	a, b, c, inside := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+b+".docj", embedTextNode(ast.NewNodeID(), "B body"))
	markdownDoc(t, m, box, "/"+c+".docj", embedTextNode(ast.NewNodeID(), "C body"))
	markdownDoc(t, m, box, "/"+a+".docj", footParagraph(inside, footRef(b, "Reference B"), footRef(b, "Duplicate"), footRef(c, "Reference C"), footRef(inside, "Inside")))
	_, trees := footPrepared(t, m, a)
	m.anchorMax = 4
	p := m.referenceRenderer()
	Conf.m.Lock()
	Conf.Editor.BlockRefDynamicAnchorTextMaxLen = 1
	Conf.m.Unlock()
	notes := []*refAsFootnotes{}
	depth := 0
	collectFootnotesDefs(trees[a], a, &notes, trees, &depth, p)
	if p.check() != nil || len(notes) != 2 || notes[0].defID != b || notes[1].defID != c || notes[0].refNum != "1" || notes[1].refNum != "2" || notes[0].refAnchorText != "Refe..." {
		t.Fatal(notes, p.check())
	}
}
func TestMarkdownExportFootnotesSingleFileRendersOriginalReferencesReadonly(t *testing.T) {
	m, box := markdownFixture(t)
	a, b, body := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+b+".docj", embedTextNode(body, "External definition body"))
	markdownDoc(t, m, box, "/"+a+".docj", footParagraph(ast.NewNodeID(), footRef(body, "Definition anchor")))
	files := []string{filepath.Join(util.DataDir, box, a+".docj"), filepath.Join(util.DataDir, box, b+".docj")}
	before := [][]byte{}
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		before = append(before, raw)
	}
	defs, trees := footPrepared(t, m, a)
	m.settings.AddTitle = false
	m.settings.MarkdownYFM = false
	_, md, _, err := exportMarkdownContent(a, ".md", 4, defs, true, trees, m)
	if err != nil || !strings.Contains(md, "[^1]") || !strings.Contains(md, "Definition anchor") || !strings.Contains(md, "External definition body") {
		t.Fatal(md, err)
	}
	for i, f := range files {
		raw, _ := os.ReadFile(f)
		if !bytes.Equal(before[i], raw) {
			t.Fatal("footnotes wrote source")
		}
	}
}
func TestMarkdownExportFootnotesQueryExpansionWrapsListItems(t *testing.T) {
	m, box := markdownFixture(t)
	root, item := ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+root+".docj", map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeList", "ListData": map[string]interface{}{"Typ": 0}, "Children": []interface{}{map[string]interface{}{"ID": item, "Type": "NodeListItem", "ListData": map[string]interface{}{"Typ": 0}, "Children": []interface{}{embedTextNode(ast.NewNodeID(), "List query body")}}}})
	embedSQLRow(t, m, item, "i")
	b, query, a := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+b+".docj", embedQueryNode(query, "SELECT * FROM blocks WHERE id='"+item+"'"))
	markdownDoc(t, m, box, "/"+a+".docj", footParagraph(ast.NewNodeID(), footRef(query, "Query anchor")))
	defs, trees := footPrepared(t, m, a)
	m.settings.AddTitle = false
	m.settings.MarkdownYFM = false
	_, md, _, err := exportMarkdownContent(a, ".md", 4, defs, true, trees, m)
	if err != nil || !strings.Contains(md, "List query body") || !strings.Contains(md, "[^1]") {
		t.Fatal(md, err)
	}
	if len(trees[a].Root.ChildrenByType(ast.NodeList)) == 0 {
		t.Fatal("list-item wrapper lost")
	}
}
func TestMarkdownExportFootnotesRecursiveMarkdownHeadingCycleAndIndependentTrees(t *testing.T) {
	m, box := markdownFixture(t)
	a, b, h := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+a+".docj", map[string]interface{}{"ID": h, "Type": "NodeHeading", "HeadingLevel": 2, "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": "Selected heading"}}}, embedTextNode(ast.NewNodeID(), "Heading body"), embedQueryNode(ast.NewNodeID(), "SELECT * FROM blocks WHERE id='"+b+"'"))
	markdownDoc(t, m, box, "/"+b+".docj", embedQueryNode(ast.NewNodeID(), "SELECT * FROM blocks WHERE id='"+a+"'"), embedTextNode(ast.NewNodeID(), "Nested cycle leaf"))
	embedSQLRow(t, m, a, "d")
	embedSQLRow(t, m, b, "d")
	p := m.referenceRenderer()
	seen := []string{}
	nodes := renderBlockMarkdownR(h, &seen, p)
	if p.check() != nil || len(nodes) < 2 || !strings.Contains(footText(t, p, nodes[0]), "Selected heading") {
		t.Fatal(len(nodes), p.check())
	}
	// A fresh per-call tree must still contain the source embed and all original nodes.
	seen = []string{}
	nodes = renderBlockMarkdownR(a, &seen, p)
	text := ""
	for _, n := range nodes {
		text += footText(t, p, n)
	}
	if p.check() != nil || !strings.Contains(text, "Nested cycle leaf") || len(seen) > 3 {
		t.Fatal(text, seen, p.check())
	}
	if again := renderBlockMarkdownR(a, &seen, p); len(again) != 0 {
		t.Fatal("rendered-set dedup changed")
	}
}
func TestMarkdownExportFootnotesSameDocumentAbbreviationUsesOwnedSQL(t *testing.T) {
	m, box := markdownFixture(t)
	root, para := ast.NewNodeID(), ast.NewNodeID()
	tree := markdownDoc(t, m, box, "/"+root+".docj", embedTextNode(para, strings.Repeat("x", 90)))
	insertMarkdownRefText(t, m, para, strings.Repeat("x", 90))
	current := &parse.Tree{ID: root, Root: &ast.Node{ID: root, Type: ast.NodeDocument}, Box: box, Path: tree.Path}
	notes := []*refAsFootnotes{{defID: para, refNum: "1", refAnchorText: "anchor"}}
	p := m.referenceRenderer()
	out := resolveFootnotesDefs(&notes, current, map[string]bool{}, "[", "]", map[string]*parse.Tree{root: tree}, p)
	text := footText(t, p, out)
	if p.check() != nil || !strings.Contains(text, strings.Repeat("x", 64)+"...") || strings.Contains(text, strings.Repeat("x", 65)) {
		t.Fatal(text, p.check())
	}
}
func TestMarkdownExportFootnotesFailuresReturnNoPartialDefinitionOrNodes(t *testing.T) {
	for _, kind := range []string{"bad-query", "write-query", "bad-source", "missing-node", "missing-script"} {
		t.Run(kind, func(t *testing.T) {
			m, box := markdownFixture(t)
			id := ast.NewNodeID()
			script := "SELECT * FROM missing_table"
			if kind == "write-query" {
				script = "DELETE FROM blocks"
			}
			node := embedQueryNode(ast.NewNodeID(), script)
			if kind == "missing-script" {
				delete(node, "Children")
			}
			tree := markdownDoc(t, m, box, "/"+id+".docj", embedTextNode(ast.NewNodeID(), "Partial must not escape"), node)
			selected := id
			if kind == "bad-source" {
				os.WriteFile(filepath.Join(util.DataDir, box, id+".docj"), []byte("bad"), 0600)
				fresh, err := captureMarkdownExport(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				m = fresh
			}
			if kind == "missing-node" {
				ghost := &ast.Node{ID: ast.NewNodeID(), Type: ast.NodeParagraph}
				tree.Root.AppendChild(ghost)
				if err := m.read.binding.scope.commit.r.registry.Upsert(context.Background(), tree); err != nil {
					t.Fatal(err)
				}
				ghost.Unlink()
				selected = ghost.ID
			}
			p := m.referenceRenderer()
			seen := []string{}
			nodes := renderBlockMarkdownR(selected, &seen, p)
			if p.check() == nil || len(nodes) != 0 {
				t.Fatal("partial/failed recursive render escaped", len(nodes), p.check())
			}
		})
	}
	// A missing definition source must not return an earlier successful footnote block.
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	tree := markdownDoc(t, m, box, "/"+id+".docj")
	notes := []*refAsFootnotes{{defID: id, refNum: "1"}, {defID: ast.NewNodeID(), refNum: "2"}}
	p := m.referenceRenderer()
	out := resolveFootnotesDefs(&notes, tree, map[string]bool{}, "", "", map[string]*parse.Tree{id: tree}, p)
	if out != nil || p.check() == nil {
		t.Fatal("partial definition returned", out, p.check())
	}
}
func TestMarkdownExportFootnotesDepthCancellationAndOwnerGuards(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	tree := markdownDoc(t, m, box, "/"+id+".docj")
	p := m.referenceRenderer()
	notes := []*refAsFootnotes{}
	depth := 4096
	collectFootnotesDefs(tree, id, &notes, map[string]*parse.Tree{id: tree}, &depth, p)
	if depth != 4097 || p.check() != nil || len(notes) != 0 {
		t.Fatal(depth, p.check())
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.read.binding.scope.ctx = ctx
	cancel()
	p = m.referenceRenderer()
	seen := []string{}
	if nodes := renderBlockMarkdownR(id, &seen, p); len(nodes) != 0 || !errors.Is(p.check(), context.Canceled) {
		t.Fatal(p.check())
	}
	fresh, err := captureMarkdownExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	old := util.TempDir
	util.TempDir = t.TempDir()
	t.Cleanup(func() { util.TempDir = old })
	p = fresh.referenceRenderer()
	if out := resolveFootnotesDefs(&notes, tree, nil, "", "", map[string]*parse.Tree{id: tree}, p); out != nil || p.check() == nil || errors.Is(p.check(), context.Canceled) {
		t.Fatal(out)
	}
}

func TestMarkdownExportFootnotesCacheKeepsRequestedBlockIdentity(t *testing.T) {
	m, box := markdownFixture(t)
	root, para, target := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	tree := markdownDoc(t, m, box, "/"+root+".docj", footParagraph(para, exportReferenceNode(target, true)))
	// Simulate blockLink2Ref's earlier transformation of the root-key candidate.
	tree.Root.FirstChild.FirstChild.TextMarkType = "block-ref"
	cache := map[string]*parse.Tree{root: tree}
	p := m.referenceRenderer()
	loaded, err := exportFootnoteLoad(para, cache, []*exportReferenceRenderer{p})
	if err != nil || loaded == tree || cache[para] != loaded || cache[root] != tree || loaded.Root.FirstChild.FirstChild.TextMarkType != "a" {
		t.Fatal("root candidate replaced original block-key semantics", err)
	}
	again, err := exportFootnoteLoad(para, cache, []*exportReferenceRenderer{p})
	if err != nil || again != loaded {
		t.Fatal("requested-key cache was not reused", err)
	}
	loaded.Path = "/changed.docj"
	if _, err = exportFootnoteLoad(para, cache, []*exportReferenceRenderer{p}); err == nil {
		t.Fatal("changed cached identity admitted")
	}
}
