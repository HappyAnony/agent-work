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
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	nethtml "golang.org/x/net/html"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"wiki-kernel/av"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

// Attribute ordering is immaterial; retain every attribute value, node and ID.
func canonicalFileHTML(t *testing.T, s string) string {
	t.Helper()
	root, err := nethtml.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		sort.Slice(n.Attr, func(i, j int) bool {
			if n.Attr[i].Namespace != n.Attr[j].Namespace {
				return n.Attr[i].Namespace < n.Attr[j].Namespace
			}
			return n.Attr[i].Key < n.Attr[j].Key
		})
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	var b bytes.Buffer
	if err = nethtml.Render(&b, root); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
func fileContextFixture(t *testing.T) (*markdownExportContext, string, map[string][]byte) {
	t.Helper()
	m, tree, _, _, before := htmlResourceFixture(t)
	box := tree.Box
	id := ast.NewNodeID()
	nativePreRenderDoc(t, m, box, "/"+id+".docj", focusHeading(ast.NewNodeID(), 4, "File heading"), footParagraph(ast.NewNodeID(), resourceLink("assets/same.pdf?download=1#page=2"), map[string]interface{}{"Type": "NodeEmojiImg", "Data": "<img src=\"/emojis/face.png\" />"}), map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeIFrame", "Data": "<iframe src=\"https://example.invalid/embed\"></iframe>"})
	filename := filepath.Join(m.read.binding.scope.commit.r.data, box, id+".docj")
	before[filename], _ = os.ReadFile(filename)
	m.settings.AddTitle = false
	return m, id, before
}
func assertFileSourceUnchanged(t *testing.T, before map[string][]byte) {
	t.Helper()
	for name, want := range before {
		got, e := os.ReadFile(name)
		if e != nil || !bytes.Equal(want, got) {
			t.Fatal("source changed", name, e)
		}
	}
}
func assertNoFileWork(t *testing.T, m *markdownExportContext) {
	t.Helper()
	if m.read.output != nil {
		t.Fatal("output still bound")
	}
	entries, _ := os.ReadDir(m.read.temp)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".wiki-archive-") {
			t.Fatal("private work leaked", e.Name())
		}
	}
}

func TestHTMLFileAssemblyBothRenderersResourcesAndPublication(t *testing.T) {
	for _, mode := range []string{"html", "pdf", "markdown", "docx"} {
		t.Run(mode, func(t *testing.T) {
			m, id, before := fileContextFixture(t)
			options := htmlExportFileOptions{PDF: mode == "pdf", Markdown: mode == "markdown" || mode == "docx", Docx: mode == "docx"}
			var zipPath string
			var borrowed *archiveExportOutput
			err := m.withHTMLFile(id, options, func(f htmlExportFile) error {
				borrowed = f.output
				if f.name == "" || !strings.Contains(f.content, "File heading") {
					t.Fatal("empty rendering")
				}
				if _, e := f.output.payload.Stat("index.html"); !os.IsNotExist(e) {
					t.Fatal("frontend wrapping bypassed", e)
				}
				if e := f.output.payload.WriteFile("index.html", []byte(f.content), 0644); e != nil {
					return e
				}
				if e := f.output.pack(); e != nil {
					return e
				}
				zipPath = filepath.Join(m.read.temp, "export", f.output.filename)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			assertNoFileWork(t, m)
			if err = borrowed.check(); err == nil {
				t.Fatal("borrow escaped lifecycle")
			}
			got := readOutputZip(t, zipPath)
			if string(got["assets/same.pdf"]) != "global PDF" || len(got["index.html"]) == 0 {
				t.Fatal("missing file payload")
			}
			if mode == "pdf" {
				if got["stage/protyle/runtime.js"] != nil || got["emojis/face.png"] != nil {
					t.Fatal("PDF copied static tree")
				}
			} else if string(got["stage/protyle/runtime.js"]) != "runtime" || string(got["emojis/face.png"]) != "custom emoji" {
				t.Fatal("static or emoji missing")
			}
			if options.Markdown && strings.Contains(string(got["index.html"]), `src="/emojis`) {
				t.Fatal("emoji URL not normalized")
			}
			if (mode == "pdf" || mode == "docx") && strings.Contains(string(got["index.html"]), "<iframe") {
				t.Fatal("iframe not converted")
			}
			assertFileSourceUnchanged(t, before)
		})
	}
}
func TestHTMLFileMarkdownValidLegacyDOMAndResources(t *testing.T) {
	for _, docx := range []bool{false, true} {
		t.Run(map[bool]string{false: "preview", true: "word"}[docx], func(t *testing.T) {
			m, id, before := fileContextFixture(t)
			oldWork, oldAppearance := util.WorkingDir, util.AppearancePath
			util.WorkingDir = m.htmlResources.work.path
			util.AppearancePath = filepath.Dir(filepath.Dir(m.htmlResources.theme.path))
			t.Cleanup(func() { util.WorkingDir, util.AppearancePath = oldWork, oldAppearance })
			Conf.m.Lock()
			Conf.Appearance = conf.NewAppearance()
			Conf.Appearance.ThemeLight = "custom"
			Conf.Appearance.Mode = 0
			Conf.Appearance.Icon = "custom"
			Conf.Export.AddTitle = false
			Conf.m.Unlock()
			legacyDir := t.TempDir()
			name, want := ExportMarkdownHTML(id, legacyDir, docx, false)
			err := m.withHTMLFile(id, htmlExportFileOptions{Markdown: true, Docx: docx}, func(f htmlExportFile) error {
				if f.name != name || canonicalFileHTML(t, f.content) != canonicalFileHTML(t, want) {
					t.Fatal("legacy DOM changed")
				}
				for _, p := range []string{"assets/same.pdf", "emojis/face.png", "stage/protyle/runtime.js", "appearance/themes/custom/theme.css", "appearance/icons/custom/icon.js", "appearance/icons/material/icon.js"} {
					a, e := os.ReadFile(filepath.Join(legacyDir, p))
					if e != nil {
						t.Fatal(e)
					}
					b, e := f.output.payload.ReadFile(p)
					if e != nil || !bytes.Equal(a, b) {
						t.Fatal("resource parity", p, e)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			assertNoFileWork(t, m)
			assertFileSourceUnchanged(t, before)
		})
	}
}
func TestHTMLFileAssemblyFailuresCleanAndSkipConsumer(t *testing.T) {
	for _, kind := range []string{"missing", "resources", "link", "canceled", "bad-options", "nil-consumer", "consumer", "published-consumer", "published-cancel", "panic"} {
		t.Run(kind, func(t *testing.T) {
			m, id, before := fileContextFixture(t)
			opt := htmlExportFileOptions{Markdown: true}
			called := false
			var published string
			switch kind {
			case "missing":
				id = ast.NewNodeID()
			case "resources":
				os.RemoveAll(filepath.Join(m.htmlResources.work.path, "stage/protyle"))
			case "link":
				os.Remove(filepath.Join(m.htmlResources.work.path, "stage/protyle/runtime.js"))
				os.Symlink(filepath.Join(t.TempDir(), "foreign"), filepath.Join(m.htmlResources.work.path, "stage/protyle/runtime.js"))
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				m.read.binding.scope.ctx = ctx
			case "bad-options":
				opt.PDF = true
			}
			consumer := func(f htmlExportFile) error {
				called = true
				if strings.HasPrefix(kind, "published-") || kind == "panic" {
					if e := f.output.payload.WriteFile("index.html", []byte(f.content), 0644); e != nil {
						return e
					}
					if e := f.output.pack(); e != nil {
						return e
					}
					published = filepath.Join(m.read.temp, "export", f.output.filename)
					if kind == "published-cancel" {
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						m.read.binding.scope.ctx = ctx
						return nil
					}
					if kind == "panic" {
						panic("synthetic consumer failure")
					}
				}
				return errors.New("synthetic consumer error")
			}
			if kind == "nil-consumer" {
				consumer = nil
			}
			var err error
			panicked := false
			func() {
				defer func() {
					if recover() != nil {
						panicked = true
					}
				}()
				err = m.withHTMLFile(id, opt, consumer)
			}()
			if kind == "panic" {
				if !panicked {
					t.Fatal("panic swallowed")
				}
			} else if err == nil {
				t.Fatal("failure hidden")
			}
			wantsCalled := kind == "consumer" || strings.HasPrefix(kind, "published-") || kind == "panic"
			if called != wantsCalled {
				t.Fatal("consumer called unexpectedly", kind, called)
			}
			if published != "" {
				if _, e := os.Stat(published); !os.IsNotExist(e) {
					t.Fatal("failed publication kept", e)
				}
			}
			assertNoFileWork(t, m)
			if kind != "resources" && kind != "link" {
				assertFileSourceUnchanged(t, before)
			}
		})
	}
}
func TestHTMLFileMarkdownMergeFocusAndCapturedPreferences(t *testing.T) {
	m, _, before := mergeFixture(t)
	m.settings.AddTitle = false
	tree, name, content, err := m.renderMarkdownHTMLDocument(mergeRoot, false, true)
	if err != nil || tree == nil || name != "Root" || !strings.Contains(content, "Alpha body") || !strings.Contains(content, "Grand body") || strings.Contains(content, "Hidden body") {
		t.Fatal(name, content, err)
	}
	original := m.settings
	Conf.m.Lock()
	Conf.Export = nil
	Conf.m.Unlock()
	_, _, again, err := m.renderMarkdownHTMLDocument(mergeRoot, false, true)
	if err != nil || !reflect.DeepEqual(m.settings, original) || canonicalFileHTML(t, normalizePreRenderTemporaryIDs(content)) != canonicalFileHTML(t, normalizePreRenderTemporaryIDs(again)) {
		t.Fatal("captured preferences lost", err)
	}
	assertFileSourceUnchanged(t, before)
	m2, box := markdownFixture(t)
	id, a, b, c := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	nativePreRenderDoc(t, m2, box, "/"+id+".docj", focusHeading(a, 2, "Selected"), footParagraph(b, map[string]interface{}{"Type": "NodeText", "Data": "inside"}), focusHeading(c, 2, "Outside"))
	_, _, focused, err := m2.renderMarkdownHTMLDocument(a, false, false)
	if err != nil || !strings.Contains(focused, "inside") || strings.Contains(focused, "Outside") {
		t.Fatal("focus changed", err)
	}
}
func TestHTMLFileMarkdownNormalizationAndGuard(t *testing.T) {
	root := &ast.Node{Type: ast.NodeDocument}
	emoji := &ast.Node{Type: ast.NodeEmojiImg, Tokens: []byte(`<img src="/emojis/f.png" />`)}
	root.AppendChild(emoji)
	list := &ast.Node{Type: ast.NodeList, ListData: &ast.ListData{Typ: 1}}
	li := &ast.Node{Type: ast.NodeListItem, ListData: &ast.ListData{Num: 7}}
	list.AppendChild(li)
	root.AppendChild(list)
	code := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "code", TextMarkTextContent: "x"}
	root.AppendChild(code)
	text := &ast.Node{Type: ast.NodeText, Tokens: []byte(editor.Zwsp + "tail")}
	root.AppendChild(text)
	tree := &parse.Tree{Root: root}
	_, err := renderMarkdownExportHTML(tree, lute.New(), false, func() error { return nil })
	if err != nil || string(emoji.Tokens) != `<img src="emojis/f.png" />` || list.ListData.Start != 7 || string(text.Tokens) != "tail" {
		t.Fatal("normalization", err)
	}
	calls := 0
	guard := func() error {
		calls++
		if calls > 1 {
			return context.Canceled
		}
		return nil
	}
	if content, e := renderMarkdownExportHTML(tree, lute.New(), false, guard); !errors.Is(e, context.Canceled) || content != "" {
		t.Fatal("partial HTML", content, e)
	}
	if content, e := renderMarkdownExportHTML(nil, nil, false, nil); e == nil || content != "" {
		t.Fatal("unbound renderer accepted")
	}
}

func TestHTMLFileMarkdownAVAndQueryFailure(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			m, _, tree, file := markdownAVFixture(t, layout)
			before, _ := os.ReadFile(file)
			_, _, content, err := m.renderMarkdownHTMLDocument(tree.ID, false, false)
			if err != nil || !strings.Contains(content, "alpha") || !strings.Contains(content, "gamma") || strings.Contains(content, "beta") {
				t.Fatal("AV filtering changed", content, err)
			}
			after, _ := os.ReadFile(file)
			if !bytes.Equal(before, after) {
				t.Fatal("AV source changed")
			}
		})
	}
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj", embedQueryNode(ast.NewNodeID(), "SELECT * FROM invalid_file_html_query"))
	called := false
	err := m.withHTMLFile(id, htmlExportFileOptions{Markdown: true}, func(htmlExportFile) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("query failure hidden", err)
	}
	assertNoFileWork(t, m)
}
