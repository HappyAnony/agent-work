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
	"regexp"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/util"
)

// Old-Spec fixtures without block IAL can legitimately acquire temporary
// paragraph IDs per detached parse. Document IDs and outline URLs stay asserted.
func normalizePreRenderTemporaryIDs(s string) string {
	return regexp.MustCompile(` (data-node-id|id)="[0-9]{14}-[a-z0-9]{7}"`).ReplaceAllString(s, "")
}
func nativePreRenderDoc(t *testing.T, m *markdownExportContext, box, p string, children ...map[string]interface{}) {
	t.Helper()
	var decorate func(map[string]interface{})
	decorate = func(n map[string]interface{}) {
		if id, ok := n["ID"].(string); ok {
			attrs, ok := n["Properties"].(map[string]string)
			if !ok {
				attrs = map[string]string{}
			}
			attrs["id"] = id
			n["Properties"] = attrs
		}
		if list, ok := n["Children"].([]interface{}); ok {
			for _, child := range list {
				if value, ok := child.(map[string]interface{}); ok {
					decorate(value)
				}
			}
		}
	}
	nodes := []interface{}{}
	for _, n := range children {
		decorate(n)
		nodes = append(nodes, n)
	}
	id := util.GetTreeID(p)
	raw, err := json.Marshal(map[string]interface{}{"ID": id, "Type": "NodeDocument", "Spec": "1", "Properties": map[string]string{"id": id, "title": "Prepared Title"}, "Children": nodes})
	if err != nil {
		t.Fatal(err)
	}
	archiveWrite(t, m.read.binding.scope.commit.r.data, box+p, raw)
	trees, err := m.read.selectedTrees(box, []string{p})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.read.binding.scope.commit.r.registry.Upsert(context.Background(), trees[id]); err != nil {
		t.Fatal(err)
	}
}

func TestHTMLPreRenderAssemblesMergeAndResponseMetadata(t *testing.T) {
	m, _, before := mergeFixture(t)
	m.settings.AddTitle = false
	result, err := m.previewExportHTML(mergeRoot, ExportHTMLPreviewOptions{Merge: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "Root" || result.Type != "NodeDocument" || result.Attributes["id"] != mergeRoot || !strings.Contains(result.Content, "Alpha body") || !strings.Contains(result.Content, "Grand body") || strings.Contains(result.Content, "Hidden body") {
		t.Fatal(result, err)
	}
	if !strings.Contains(result.Content, PdfOutlineScheme+"://"+mergeA) || !strings.Contains(result.Content, PdfOutlineScheme+"://"+mergeGrand) {
		t.Fatal("outline markers missing")
	}
	result.Attributes["id"] = "caller edit"
	again, err := m.previewExportHTML(mergeRoot, ExportHTMLPreviewOptions{Image: true, Merge: true})
	if err != nil || again.Attributes["id"] != mergeRoot || normalizePreRenderTemporaryIDs(again.Content) != normalizePreRenderTemporaryIDs(result.Content) {
		t.Fatal("response aliased or image semantics changed", err)
	}
	for file, want := range before {
		raw, _ := os.ReadFile(file)
		if !bytes.Equal(raw, want) {
			t.Fatal("source modified", file)
		}
	}
	if m.read.output != nil {
		t.Fatal("pre-render published output")
	}
}
func TestHTMLPreRenderOwnedMatchesLegacyValidDOM(t *testing.T) {
	m, box := markdownFixture(t)
	m.settings.AddTitle = false
	Conf.m.Lock()
	Conf.Export.AddTitle = false
	Conf.m.Unlock()
	id, heading, quoted := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	nativePreRenderDoc(t, m, box, "/"+id+".docj", focusHeading(heading, 3, "Outline heading"), map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeBlockquote", "Children": []interface{}{map[string]interface{}{"Type": "NodeBlockquoteMarker", "Data": ">"}, focusHeading(quoted, 2, "Quoted heading")}}, map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeIFrame", "Data": "<iframe src=\"https://example.invalid/?a=1&amp;b=2\"></iframe>"}, map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeHTMLBlock", "Data": "<style>.owned { color: red; }</style><div>Raw HTML retained</div>"})
	before, _ := os.ReadFile(filepath.Join(m.read.binding.scope.commit.r.data, box, id+".docj"))
	name, content, node := ExportHTML(id, "", true, false, false, false)
	result, err := m.previewExportHTML(id, ExportHTMLPreviewOptions{})
	if err != nil || name != result.Name || node.Type.String() != result.Type || content != result.Content {
		t.Fatal("legacy DOM differs", err)
	}
	if !strings.Contains(result.Content, "&lt;style&gt;") || !strings.Contains(result.Content, "Raw HTML retained") || !strings.Contains(result.Content, "example.invalid") || strings.Contains(result.Content, "<iframe") {
		t.Fatal("original HTML or iframe rules changed")
	}
	if !strings.Contains(result.Content, PdfOutlineScheme+"://"+heading) || strings.Contains(result.Content, PdfOutlineScheme+"://"+quoted) {
		t.Fatal("quoted heading outline rule changed")
	}
	after, _ := os.ReadFile(filepath.Join(m.read.binding.scope.commit.r.data, box, id+".docj"))
	if !bytes.Equal(before, after) {
		t.Fatal("source changed")
	}
}
func TestHTMLPreRenderFocusFoldAndCapturedOrigin(t *testing.T) {
	oldPort := util.ServerPort
	util.ServerPort = "777"
	t.Cleanup(func() { util.ServerPort = oldPort })
	m, box := markdownFixture(t)
	m.settings.AddTitle = false
	id, h, p := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj", embedTextNode(ast.NewNodeID(), "Before selected"), focusHeading(h, 4, "Selected heading"), map[string]interface{}{"ID": p, "Type": "NodeParagraph", "Properties": map[string]string{"fold": "1"}, "Children": []interface{}{resourceLink(m.htmlLocalAnchorPrefix + "note"), map[string]interface{}{"Type": "NodeText", "Data": "Selected body"}}}, focusHeading(ast.NewNodeID(), 4, "Peer heading"))
	Conf.m.Lock()
	Conf.Export = nil
	Conf.m.Unlock()
	util.ServerPort = "999"
	result, err := m.previewExportHTML(h, ExportHTMLPreviewOptions{KeepFold: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "NodeHeading" || result.Attributes["id"] != h || !strings.Contains(result.Content, "Selected body") || strings.Contains(result.Content, "Before selected") || strings.Contains(result.Content, "Peer heading") || !strings.Contains(result.Content, "href=\"#note\"") || !strings.Contains(result.Content, "fold=\"1\"") {
		t.Fatal(result)
	}
	unfolded, err := m.previewExportHTML(h, ExportHTMLPreviewOptions{})
	if err != nil || strings.Contains(unfolded.Content, "fold=\"1\"") {
		t.Fatal(unfolded, err)
	}
}
func TestHTMLPreRenderAVThreeLayoutFilteringAndNoSourceWrite(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			m, _, tree, file := markdownAVFixture(t, layout)
			before, _ := os.ReadFile(file)
			result, err := m.previewExportHTML(tree.ID, ExportHTMLPreviewOptions{})
			if err != nil || !strings.Contains(result.Content, "gamma") || !strings.Contains(result.Content, "alpha") || strings.Contains(result.Content, "beta") {
				t.Fatal(result, err)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("AV source modified")
			}
		})
	}
}
func TestHTMLPreRenderFailuresNeverReturnPartialResult(t *testing.T) {
	for _, kind := range []string{"query", "iframe", "canceled", "missing", "closed"} {
		t.Run(kind, func(t *testing.T) {
			m, box := markdownFixture(t)
			id := ast.NewNodeID()
			node := embedTextNode(ast.NewNodeID(), "prefix body")
			if kind == "query" {
				node = embedQueryNode(ast.NewNodeID(), "SELECT * FROM invalid_prerender_table")
			}
			if kind == "iframe" {
				node = map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeIFrame", "Data": "<iframe src=\"unterminated"}
			}
			markdownDoc(t, m, box, "/"+id+".docj", node)
			switch kind {
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				m.read.binding.scope.ctx = ctx
			case "missing":
				id = ast.NewNodeID()
			case "closed":
				owner := m.read.binding.scope.commit.r.owner
				owner.repoMu.Lock()
				owner.repoClosed = true
				owner.repoMu.Unlock()
				t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
			}
			result, err := m.previewExportHTML(id, ExportHTMLPreviewOptions{})
			if err == nil || !reflect.DeepEqual(result, ExportHTMLPreviewResult{}) {
				t.Fatal(result, err)
			}
		})
	}
	if result, err := ExportPreviewHTMLContext(nil, ast.NewNodeID(), ExportHTMLPreviewOptions{}); err == nil || result.Content != "" {
		t.Fatal(result, err)
	}
}
func TestHTMLPreRenderSharedHelpersGuardsAndIframeTextFallback(t *testing.T) {
	m, _ := markdownFixture(t)
	root := &ast.Node{Type: ast.NodeDocument}
	frame := &ast.Node{Type: ast.NodeIFrame, Tokens: []byte("<iframe>without source</iframe>")}
	root.AppendChild(frame)
	tree := &parse.Tree{Root: root}
	if err := processExportIFrames(tree, m.guard); err != nil || frame.Type != ast.NodeParagraph || frame.FirstChild == nil || string(frame.FirstChild.Tokens) != "<iframe>without source</iframe>" {
		t.Fatal(err)
	}
	calls := 0
	guard := func() error {
		calls++
		if calls > 1 {
			return context.Canceled
		}
		return nil
	}
	root.AppendChild(&ast.Node{Type: ast.NodeHeading, ID: ast.NewNodeID(), HeadingLevel: 2})
	if err := addExportPDFOutline(tree, guard); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	calls = 0
	if html, err := renderExportHTML(tree, m.referenceRenderer().parser(), guard); !errors.Is(err, context.Canceled) || html != "" {
		t.Fatal(html, err)
	}
	if _, err := renderExportHTML(nil, nil, nil); err == nil {
		t.Fatal("nil rendering dependencies accepted")
	}
	if err := processExportIFrames(nil, nil); err == nil {
		t.Fatal("nil iframe dependencies accepted")
	}
}
