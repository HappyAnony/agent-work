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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/util"
)

func embedQueryNode(id, stmt string) map[string]interface{} {
	return map[string]interface{}{"ID": id, "Type": "NodeBlockQueryEmbed", "Children": []interface{}{map[string]interface{}{"Type": "NodeOpenBrace"}, map[string]interface{}{"Type": "NodeOpenBrace"}, map[string]interface{}{"Type": "NodeBlockQueryEmbedScript", "Data": stmt}, map[string]interface{}{"Type": "NodeCloseBrace"}, map[string]interface{}{"Type": "NodeCloseBrace"}}}
}
func embedTextNode(id, text string) map[string]interface{} {
	return map[string]interface{}{"ID": id, "Type": "NodeParagraph", "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": text}}}
}
func embedSQLRow(t *testing.T, m *markdownExportContext, id, kind string) {
	t.Helper()
	insertMarkdownRefText(t, m, id, "embed indexed text")
	if _, err := m.read.binding.scope.commit.r.main.DB.Exec("UPDATE blocks SET type=? WHERE id=?", kind, id); err != nil {
		t.Fatal(err)
	}
}
func resolveEmbeddedText(t *testing.T, m *markdownExportContext, id string, mode int) (string, *exportReferenceRenderer) {
	t.Helper()
	tree, err := m.embeddedTree(id)
	if err != nil {
		t.Fatal(err)
	}
	p := m.referenceRenderer()
	depth := 0
	resolveEmbedR(tree.Root, mode, p.parser(), &[]string{}, &depth, p)
	text, err := lute.FormatNodeSync(tree.Root, p.parser().ParseOptions, p.parser().RenderOptions)
	if err != nil {
		t.Fatal(err)
	}
	return text, p
}
func TestMarkdownExportEmbedCapturesQueryAndHeadingSettings(t *testing.T) {
	m, _ := markdownFixture(t)
	a, b := ast.NewNodeID(), ast.NewNodeID()
	embedSQLRow(t, m, a, "p")
	embedSQLRow(t, m, b, "p")
	Conf.m.Lock()
	Conf.Search.Limit = 1
	Conf.Editor.HeadingEmbedMode = 2
	Conf.m.Unlock()
	c, err := captureMarkdownExport(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	Conf.m.Lock()
	Conf.Search.Limit = 99
	Conf.Editor.HeadingEmbedMode = 0
	Conf.m.Unlock()
	rows, err := c.selectEmbeddedBlocks("SELECT * FROM blocks ORDER BY id")
	if err != nil || len(rows) != 1 || c.headingMode != 2 {
		t.Fatal(rows, err, c.headingMode)
	}
	if _, err = c.selectEmbeddedBlocks("DELETE FROM blocks"); err == nil {
		t.Fatal("write query admitted")
	}
	rows, err = m.selectEmbeddedBlocks("SELECT * FROM blocks WHERE id IN ('" + a + "','" + b + "') ORDER BY id")
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	Conf.m.Lock()
	Conf.Search = nil
	Conf.m.Unlock()
	if _, err = captureMarkdownExport(context.Background()); err == nil {
		t.Fatal("missing Search admitted")
	}
}
func TestMarkdownExportEmbedTreeCopiesFirstReadAndKeepsSourceBytes(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj", embedTextNode(ast.NewNodeID(), "Original embed text"))
	file := filepath.Join(util.DataDir, box, id+".docj")
	before, _ := os.ReadFile(file)
	first, err := m.embeddedTree(id)
	if err != nil {
		t.Fatal(err)
	}
	first.Root.FirstChild.FirstChild.Tokens = []byte("mutated candidate")
	second, err := m.embeddedTree(id)
	if err != nil || second == first || second.Root.FirstChild.FirstChild.TokensStr() != "Original embed text" {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(file); !bytes.Equal(before, raw) {
		t.Fatal("clone wrote source")
	}
	if err = os.WriteFile(file, []byte("later external version"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := m.embeddedTree(id)
	if err != nil || third.Root.FirstChild.FirstChild.TokensStr() != "Original embed text" {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(file); string(raw) != "later external version" {
		t.Fatal("overwrote later external source")
	}
}
func TestMarkdownExportEmbedDocumentRawQuoteAndIndependentHeadingAdjustment(t *testing.T) {
	m, box := markdownFixture(t)
	target, heading, body := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+target+".docj", map[string]interface{}{"ID": heading, "Type": "NodeHeading", "HeadingLevel": 1, "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": "Embedded heading"}}}, embedTextNode(body, "Embedded body"))
	embedSQLRow(t, m, target, "d")
	source := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+source+".docj", map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeHeading", "HeadingLevel": 3, "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": "Parent"}}}, embedQueryNode(ast.NewNodeID(), "SELECT * FROM blocks WHERE id='"+target+"'"), embedQueryNode(ast.NewNodeID(), "SELECT * FROM blocks WHERE id='"+target+"'"))
	before, _ := os.ReadFile(filepath.Join(util.DataDir, box, target+".docj"))
	raw, p := resolveEmbeddedText(t, m, source, 0)
	if p.check() != nil || strings.Count(raw, "#### Embedded heading") != 2 || strings.Count(raw, "Embedded body") != 2 {
		t.Fatal(raw, p.check())
	}
	quoted, p := resolveEmbeddedText(t, m, source, 1)
	if p.check() != nil || !strings.Contains(quoted, "> # Embedded heading") || !strings.Contains(quoted, "> Embedded body") {
		t.Fatal(quoted, p.check())
	}
	after, _ := os.ReadFile(filepath.Join(util.DataDir, box, target+".docj"))
	if !bytes.Equal(before, after) {
		t.Fatal("embedded heading wrote source")
	}
}
func TestMarkdownExportEmbedHeadingModesAndOverride(t *testing.T) {
	m, box := markdownFixture(t)
	target, h := ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+target+".docj", map[string]interface{}{"ID": h, "Type": "NodeHeading", "HeadingLevel": 2, "Children": []interface{}{map[string]interface{}{"Type": "NodeText", "Data": "Heading selection"}}}, embedTextNode(ast.NewNodeID(), "Following body"))
	embedSQLRow(t, m, h, "h")
	for _, mode := range []int{0, 1, 2} {
		m.headingMode = mode
		id := ast.NewNodeID()
		markdownDoc(t, m, box, "/"+id+".docj", embedQueryNode(ast.NewNodeID(), "SELECT * FROM blocks WHERE id='"+h+"'"))
		text, p := resolveEmbeddedText(t, m, id, 0)
		if p.check() != nil || strings.Contains(text, "Heading selection") != (mode != 2) || strings.Contains(text, "Following body") != (mode != 1) {
			t.Fatal(mode, text, p.check())
		}
	}
	m.headingMode = 2
	id := ast.NewNodeID()
	node := embedQueryNode(ast.NewNodeID(), "SELECT * FROM blocks WHERE id='"+h+"'")
	node["Properties"] = map[string]string{"custom-heading-mode": "1"}
	markdownDoc(t, m, box, "/"+id+".docj", node)
	text, p := resolveEmbeddedText(t, m, id, 0)
	if p.check() != nil || !strings.Contains(text, "Heading selection") || strings.Contains(text, "Following body") {
		t.Fatal(text, p.check())
	}
}
func TestMarkdownExportEmbedNestedQueriesAndDepthBoundary(t *testing.T) {
	m, box := markdownFixture(t)
	target := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+target+".docj", embedTextNode(ast.NewNodeID(), "Nested leaf"))
	embedSQLRow(t, m, target, "d")
	middle := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+middle+".docj", embedQueryNode(ast.NewNodeID(), "SELECT * FROM blocks WHERE id='"+target+"'"))
	embedSQLRow(t, m, middle, "d")
	source := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+source+".docj", embedQueryNode(ast.NewNodeID(), "SELECT * FROM blocks WHERE id='"+middle+"'"))
	text, p := resolveEmbeddedText(t, m, source, 0)
	if p.check() != nil || !strings.Contains(text, "Nested leaf") {
		t.Fatal(text, p.check())
	}
	tree, err := m.embeddedTree(source)
	if err != nil {
		t.Fatal(err)
	}
	depth := 7
	p = m.referenceRenderer()
	resolved := []string{}
	resolveEmbedR(tree.Root, 0, p.parser(), &resolved, &depth, p)
	if depth != 8 || len(resolved) != 0 || tree.Root.FirstChild.Type != ast.NodeBlockQueryEmbed || p.check() != nil {
		t.Fatal(depth, resolved, p.check())
	}
}
func TestMarkdownExportEmbedFailuresReachOwnedEntry(t *testing.T) {
	for _, kind := range []string{"query", "source", "missing-node", "malformed-script", "cancel", "temp"} {
		t.Run(kind, func(t *testing.T) {
			m, box := markdownFixture(t)
			target := ast.NewNodeID()
			markdownDoc(t, m, box, "/"+target+".docj")
			embedSQLRow(t, m, target, "p")
			if kind == "missing-node" {
				tree := m.read.documents[target].tree
				missing := &ast.Node{ID: ast.NewNodeID(), Type: ast.NodeParagraph}
				tree.Root.AppendChild(missing)
				if err := m.read.binding.scope.commit.r.registry.Upsert(context.Background(), tree); err != nil {
					t.Fatal(err)
				}
				missing.Unlink()
				target = missing.ID
				embedSQLRow(t, m, target, "p")
			}

			stmt := "SELECT * FROM blocks WHERE id='" + target + "'"
			if kind == "query" {
				stmt = "SELECT * FROM no_such_table"
			}
			source := ast.NewNodeID()
			node := embedQueryNode(ast.NewNodeID(), stmt)
			if kind == "malformed-script" {
				delete(node, "Children")
			}
			markdownDoc(t, m, box, "/"+source+".docj", node)
			if kind == "source" {
				os.WriteFile(filepath.Join(util.DataDir, box, target+".docj"), []byte("bad"), 0600)
				fresh, err := captureMarkdownExport(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				m = fresh
			}
			tree, err := m.embeddedTree(source)
			if err != nil {
				t.Fatal(err)
			}
			p := m.referenceRenderer()
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
			depth := 0
			resolveEmbedR(tree.Root, 0, p.parser(), &[]string{}, &depth, p)
			if p.check() == nil {
				t.Fatal("failure became empty embed")
			}
			if kind == "cancel" && !errors.Is(p.check(), context.Canceled) {
				t.Fatal(p.check())
			}
		})
	}
}
func TestMarkdownExportEmbedFailureSuppressesZIPAndCleansScratch(t *testing.T) {
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj", embedQueryNode(ast.NewNodeID(), "SELECT * FROM no_such_table"))
	old := archiveWrite(t, m.read.temp, "export/Prepared Title.md.zip", []byte("prior"))
	name, url := ExportPandocConvertZip([]string{id}, "", ".md", context.Background())
	if name == "" || url != "" {
		t.Fatal(name, url)
	}
	raw, _ := os.ReadFile(old)
	if string(raw) != "prior" {
		t.Fatal("old output overwritten")
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
