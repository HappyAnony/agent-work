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
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func focusHeading(id string, level int, text string) map[string]interface{} {
	return map[string]interface{}{"ID": id, "Type": "NodeHeading", "HeadingLevel": level, "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": text}}}
}
func focusFixture(t *testing.T) (*markdownExportContext, string, []string) {
	t.Helper()
	m, box := markdownFixture(t)
	ids := []string{}
	for i := 0; i < 7; i++ {
		ids = append(ids, ast.NewNodeID())
	}
	markdownDoc(t, m, box, "/"+ids[0]+".docj", embedTextNode(ids[1], "Before"), focusHeading(ids[2], 4, "Focused heading"), embedTextNode(ids[3], "Focused body"), focusHeading(ids[4], 6, "Nested heading"), embedTextNode(ids[5], "Nested body"), focusHeading(ids[6], 4, "Next peer"))
	return m, box, ids
}
func focusKnownIDs(tree *parse.Tree, known []string) []string {
	out := []string{}
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if entering && n != tree.Root {
			for _, id := range known {
				if n.ID == id {
					out = append(out, id)
				}
			}
		}
		return ast.WalkContinue
	})
	return out
}
func TestMarkdownExportFocusOriginalRangesAndIndependentSource(t *testing.T) {
	m, box, ids := focusFixture(t)
	file := filepath.Join(util.DataDir, box, ids[0]+".docj")
	before, _ := os.ReadFile(file)
	cases := []struct {
		id   string
		want []string
	}{{ids[0], ids[1:]}, {ids[2], ids[2:6]}, {ids[3], []string{ids[3]}}}
	for _, item := range cases {
		tree, bt, err := m.prepareFocusedExport(item.id)
		if err != nil {
			t.Fatal(err)
		}
		if got := focusKnownIDs(tree, ids); !reflect.DeepEqual(got, item.want) {
			t.Fatal(got, item.want)
		}
		if tree.ID != ids[0] || tree.Root.ID != ids[0] || tree.Box != box || tree.Root.Box != box || tree.Path != bt.Path || tree.HPath != bt.HPath {
			t.Fatal("candidate location mismatch")
		}
		// The shared legacy selector must choose exactly the same source node IDs.
		full, err := m.embeddedTree(item.id)
		if err != nil {
			t.Fatal(err)
		}
		legacy := selectExportTree(bt, full, m.referenceRenderer().parser())
		if !reflect.DeepEqual(focusKnownIDs(legacy, ids), item.want) {
			t.Fatal("legacy range changed")
		}
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) || !reflect.DeepEqual(focusKnownIDs(m.read.documents[ids[0]].tree, ids), ids[1:]) {
		t.Fatal("focused preparation altered source/cache")
	}
}
func TestMarkdownExportFocusOwnedTitleAndHeadingAdjustment(t *testing.T) {
	for _, addTitle := range []bool{false, true} {
		t.Run(map[bool]string{false: "without-title", true: "with-title"}[addTitle], func(t *testing.T) {
			m, _, ids := focusFixture(t)
			tree, _, err := m.prepareFocusedExport(ids[2])
			if err != nil {
				t.Fatal(err)
			}
			p := m.referenceRenderer()
			Conf.m.Lock()
			Conf.Export.AddTitle = !addTitle
			Conf.m.Unlock()
			text := exportMarkdownContent0(ids[2], tree, "", false, true, false, ".md", 3, 0, 0, "#", "#", "", "", addTitle, false, nil, true, false, map[string]*parse.Tree{ids[0]: tree}, p)
			if p.check() != nil || !strings.Contains(text, "Focused body") || strings.Contains(text, "Before") || strings.Contains(text, "Next peer") {
				t.Fatal(text, p.check())
			}
			headings := tree.Root.ChildrenByType(ast.NodeHeading)
			want := 1
			if addTitle {
				want = 2
				if headings[0].HeadingLevel != 1 || !strings.Contains(text, "Prepared Title") {
					t.Fatal(text)
				}
			}
			found := false
			for _, h := range headings {
				if h.ID == ids[2] {
					found = true
					if h.HeadingLevel != want {
						t.Fatal("focused heading not promoted", h.HeadingLevel, want)
					}
				}
			}
			if !found {
				t.Fatal("focused heading lost")
			}
		})
	}
}
func TestMarkdownExportFocusRegisteredCandidatesRejectAliasesAndMutation(t *testing.T) {
	for _, kind := range []string{"foreign-copy", "root", "id", "box", "path", "hpath", "root-type"} {
		t.Run(kind, func(t *testing.T) {
			m, _, ids := focusFixture(t)
			tree, _, err := m.prepareFocusedExport(ids[2])
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "foreign-copy":
				copy := *tree
				tree = &copy
			case "root":
				copy := *tree.Root
				tree.Root = &copy
			case "id":
				tree.ID = ast.NewNodeID()
			case "box":
				tree.Box = "other"
			case "path":
				tree.Path = "/other.docj"
			case "hpath":
				tree.HPath = "/changed"
			case "root-type":
				tree.Root.Type = ast.NodeParagraph
			}
			if err = m.checkExportRenderTree(tree); err == nil {
				t.Fatal("unowned/changed focus admitted")
			}
			if _, err = m.exportDocumentTitle(tree); err == nil {
				t.Fatal("title bypassed binding")
			}
		})
	}
}
func TestMarkdownExportFocusSourceFailureAndGuardErrors(t *testing.T) {
	for _, kind := range []string{"missing", "bad-source", "cancel", "temp", "closed-registry"} {
		t.Run(kind, func(t *testing.T) {
			m, box, ids := focusFixture(t)
			id := ids[2]
			if kind == "missing" {
				id = ast.NewNodeID()
			}
			if kind == "bad-source" {
				os.WriteFile(filepath.Join(util.DataDir, box, ids[0]+".docj"), []byte("bad"), 0600)
				fresh, err := captureMarkdownExport(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				m = fresh
			}
			if kind == "cancel" {
				ctx, cancel := context.WithCancel(context.Background())
				m.read.binding.scope.ctx = ctx
				cancel()
			}
			if kind == "temp" {
				old := util.TempDir
				util.TempDir = t.TempDir()
				t.Cleanup(func() { util.TempDir = old })
			}
			if kind == "closed-registry" {
				m.read.binding.scope.commit.r.registry.DB.Close()
			}
			tree, bt, err := m.prepareFocusedExport(id)
			if err == nil || tree != nil || bt != nil {
				t.Fatal(tree, bt, err)
			}
		})
	}
}
func TestMarkdownExportFocusHeadingPureRulesEmptyAndClamp(t *testing.T) {
	m, _, _ := focusFixture(t)
	p := m.referenceRenderer()
	bt := &treenode.BlockTree{Type: "h"}
	empty := &parse.Tree{Root: &ast.Node{Type: ast.NodeDocument}}
	adjustExportHeading(bt, empty, true, []*exportReferenceRenderer{p})
	if p.check() != nil {
		t.Fatal(p.check())
	}
	root := &ast.Node{Type: ast.NodeDocument}
	docTitle := &ast.Node{Type: ast.NodeHeading, HeadingLevel: 1}
	first := &ast.Node{Type: ast.NodeHeading, HeadingLevel: 1}
	deep := &ast.Node{Type: ast.NodeHeading, HeadingLevel: 6}
	root.AppendChild(docTitle)
	root.AppendChild(first)
	root.AppendChild(deep)
	tree := &parse.Tree{Root: root}
	adjustExportHeading(bt, tree, true, []*exportReferenceRenderer{p})
	if first.HeadingLevel != 2 || deep.HeadingLevel != 6 || docTitle.HeadingLevel != 1 {
		t.Fatal(first.HeadingLevel, deep.HeadingLevel)
	}
	root = &ast.Node{Type: ast.NodeDocument}
	quote := &ast.Node{Type: ast.NodeBlockquote}
	quoted := &ast.Node{Type: ast.NodeHeading, HeadingLevel: 5}
	quote.AppendChild(quoted)
	root.AppendChild(quote)
	h := &ast.Node{Type: ast.NodeHeading, HeadingLevel: 4}
	root.AppendChild(h)
	tree = &parse.Tree{Root: root}
	adjustExportHeading(bt, tree, false, []*exportReferenceRenderer{p})
	if h.HeadingLevel != 1 || quoted.HeadingLevel != 5 {
		t.Fatal(h.HeadingLevel, quoted.HeadingLevel)
	}
	adjustExportHeading(&treenode.BlockTree{Type: "d"}, tree, false, []*exportReferenceRenderer{p})
	if h.HeadingLevel != 1 {
		t.Fatal("whole document adjusted")
	}
}
func TestMarkdownExportFocusOutsideRangeDefinitionsKeepFullSourceReadable(t *testing.T) {
	m, _, ids := focusFixture(t)
	focused, _, err := m.prepareFocusedExport(ids[2])
	if err != nil {
		t.Fatal(err)
	}
	p := m.referenceRenderer()
	cache := map[string]*parse.Tree{ids[0]: focused}
	outside, err := exportRenderLoad(ids[1], cache, []*exportReferenceRenderer{p})
	if err != nil || outside == focused || treenode.GetNodeInTree(outside, ids[1]) == nil || cache[ids[0]] != focused {
		t.Fatal("outside definition reused/truncated focus", err)
	}
	inside, err := exportRenderLoad(ids[3], cache, []*exportReferenceRenderer{p})
	if err != nil || inside != focused {
		t.Fatal("focus candidate not reused for in-range read", err)
	}
}
func TestMarkdownExportFocusSameDocumentFootnoteAndCallerMetadataCopy(t *testing.T) {
	m, box := markdownFixture(t)
	root, selected, outside := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+root+".docj", footParagraph(selected, footRef(outside, "Sibling anchor")), embedTextNode(outside, "Sibling definition outside focus"))
	tree, bt, err := m.prepareFocusedExport(selected)
	if err != nil {
		t.Fatal(err)
	}
	bt.Path = "/caller-changed.docj"
	if err = m.checkExportRenderTree(tree); err != nil {
		t.Fatal("caller metadata aliases binding", err)
	}
	p := m.referenceRenderer()
	text := exportMarkdownContent0(selected, tree, "", false, false, false, ".md", 4, 0, 0, "#", "#", "", "", false, false, nil, true, false, map[string]*parse.Tree{root: tree}, p)
	if p.check() != nil || !strings.Contains(text, "[^1]") || !strings.Contains(text, "Sibling definition outside focus") {
		t.Fatal(text, p.check())
	}
}
