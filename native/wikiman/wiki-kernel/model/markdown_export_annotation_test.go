//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/util"
)

func annotationAsset(t *testing.T, m *markdownExportContext, relative, sidecar string) {
	t.Helper()
	archiveWrite(t, util.DataDir, relative, []byte("PDF bytes"))
	archiveWrite(t, util.DataDir, relative+".sya", []byte(sidecar))
}
func annotationNode(asset, id string) map[string]interface{} {
	return map[string]interface{}{"Type": "NodeTextMark", "TextMarkType": "file-annotation-ref", "TextMarkFileAnnotationRefID": asset + "/" + id, "TextMarkTextContent": "Quoted text"}
}
func TestMarkdownExportAnnotationDisplayNamesAndOriginalModes(t *testing.T) {
	m, _ := markdownFixture(t)
	asset := "assets/Report-" + ast.NewNodeID() + ".pdf"
	annotationAsset(t, m, asset, `{"a":{"pages":[{"index":2}]}}`)
	for _, mode := range []int{0, 1} {
		n := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "file-annotation-ref", TextMarkTextContent: "Quoted text"}
		parent := &ast.Node{Type: ast.NodeParagraph}
		parent.AppendChild(n)
		p := m.referenceRenderer()
		processFileAnnotationRef(asset+"/a", n, mode, p)
		if p.check() != nil || n.Previous == nil {
			t.Fatal(p.check())
		}
		link := n.Previous
		text := link.ChildByType(ast.NodeLinkText).TokensStr()
		want := "Quoted text"
		if mode == 0 {
			want = "Report.pdf - p3 - Quoted text"
		}
		if text != want || link.ChildByType(ast.NodeLinkDest).TokensStr() != asset+"#page=3" {
			t.Fatal(text, link)
		}
	}
	for _, short := range []string{"assets/a.pdf", "assets/nonstandard-name.pdf"} {
		if got := annotationDisplayName(short); got != strings.TrimPrefix(short, "assets/") {
			t.Fatal(got)
		}
	}
}
func TestMarkdownExportAnnotationPrecedenceAndMatchingResource(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(map[bool]string{false: "notebook-first", true: "global-first"}[global], func(t *testing.T) {
			m, box := markdownFixture(t)
			other := ast.NewNodeID()
			m.read.assetBoxes = append(m.read.assetBoxes, other)
			asset := "assets/shared.pdf"
			annotationAsset(t, m, box+"/"+asset, `{"a":{"pages":[{"index":0}]}}`)
			annotationAsset(t, m, other+"/"+asset, `{"a":{"pages":[{"index":7}]}}`)
			archiveWrite(t, util.DataDir, box+"/"+asset, []byte("first PDF"))
			archiveWrite(t, util.DataDir, other+"/"+asset, []byte("last PDF"))
			want, wantPage := "first PDF", 1
			if global {
				annotationAsset(t, m, asset, `{"a":{"pages":[{"index":2}]}}`)
				want, wantPage = "PDF bytes", 3
			}
			_, page, err := m.readAnnotationPage(asset + "/a")
			if err != nil || page != wantPage {
				t.Fatal(page, err)
			}
			o, err := m.newOutput("Annotation", ".md")
			if err != nil {
				t.Fatal(err)
			}
			defer o.finish(false)
			r, err := m.openResources(o)
			if err != nil {
				t.Fatal(err)
			}
			defer r.close()
			if !global && r.index[asset] != other+"/"+asset {
				t.Fatal("ordinary resource policy unexpectedly changed", r.index[asset])
			}
			if err = r.copyAsset(asset+"#page=1", "."); err != nil {
				t.Fatal(err)
			}
			raw, err := o.payload.ReadFile(asset)
			if err != nil || string(raw) != want {
				t.Fatal(string(raw), err)
			}
		})
	}
}
func TestMarkdownExportAnnotationCheckedEnumerationBeforeSelection(t *testing.T) {
	primary := errors.New("markdown export annotation enumeration incomplete")
	for _, kind := range []string{"incomplete", "replaced", "early-match"} {
		t.Run(kind, func(t *testing.T) {
			m, box := markdownFixture(t)
			asset := "assets/checked.pdf"
			annotationAsset(t, m, box+"/"+asset, `{"a":{"pages":[{"index":0}]}}`)
			archiveWrite(t, util.DataDir, box+"/z/unvisited", []byte("unvisited"))
			root, err := openArchivePublicationSource(m.read.binding.scope.commit.r.data)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			source := indexrepair.Source{Data: root}
			visitedLate := false
			op := markdownExportAnnotationIO{readDir: func(ctx context.Context, source indexrepair.Source, folder string) ([]fs.DirEntry, error) {
				if folder == box+"/z" {
					visitedLate = true
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
				}
				if folder != box || kind == "early-match" {
					return source.ReadDir(ctx, folder)
				}
				if kind == "incomplete" {
					return nil, errors.Join(indexrepair.ErrSourceReadIncomplete, os.ErrNotExist, primary)
				}
				entries, readErr := source.ReadDir(ctx, folder)
				if readErr != nil {
					return nil, readErr
				}
				if err := source.Data.Rename(folder, folder+".old"); err != nil {
					return nil, err
				}
				if err := source.Data.Mkdir(folder, 0700); err != nil {
					return nil, err
				}
				return entries, nil
			}}
			selected, err := m.findAnnotationAssetWithIO(source, asset, op)
			if kind == "early-match" {
				if err != nil || selected != box+"/"+asset || visitedLate {
					t.Fatal("early annotation match did not preserve lexical short circuit", selected, visitedLate, err)
				}
				return
			}
			if err == nil || selected != "" {
				t.Fatal("failed annotation enumeration admitted selection", selected, err)
			}
			if kind == "incomplete" {
				if !errors.Is(err, primary) || !errors.Is(err, indexrepair.ErrSourceReadIncomplete) || indexrepair.IsSourceNotExist(err) {
					t.Fatal("incomplete annotation enumeration error lost or admitted as missing", err)
				}
			} else if !errors.Is(err, indexrepair.ErrSourceChanged) {
				t.Fatal("replaced annotation directory accepted", err)
			}
		})
	}
}
func TestMarkdownExportAnnotationSidecarFirstReadAcrossIDs(t *testing.T) {
	m, _ := markdownFixture(t)
	asset := "assets/notes.pdf"
	annotationAsset(t, m, asset, `{"a":{"pages":[{"index":0}]},"b":{"pages":[{"index":3}]},"unrelated":"ignored"}`)
	if _, page, err := m.readAnnotationPage(asset + "/a"); err != nil || page != 1 {
		t.Fatal(page, err)
	}
	changed := []byte(`{"a":{"pages":[{"index":9}]},"b":{"pages":[{"index":19}]}}`)
	file := filepath.Join(util.DataDir, asset+".sya")
	if err := os.WriteFile(file, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, page, err := m.readAnnotationPage(asset + "/b"); err != nil || page != 4 {
		t.Fatal(page, err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(changed, after) {
		t.Fatal("later source overwritten")
	}
}
func TestMarkdownExportAnnotationInvalidDataReturnsErrors(t *testing.T) {
	for name, body := range map[string]string{"bad": "{bad", "null": "null", "array": "[]", "missing": `{}`, "record": `{"a":5}`, "pages": `{"a":{"pages":[]}}`, "page": `{"a":{"pages":[{}]}}`, "string": `{"a":{"pages":[{"index":"1"}]}}`, "negative": `{"a":{"pages":[{"index":-1}]}}`, "fraction": `{"a":{"pages":[{"index":1.5}]}}`, "huge": `{"a":{"pages":[{"index":1e100}]}}`} {
		t.Run(name, func(t *testing.T) {
			m, _ := markdownFixture(t)
			asset := "assets/bad.pdf"
			annotationAsset(t, m, asset, body)
			if _, _, err := m.readAnnotationPage(asset + "/a"); err == nil {
				t.Fatal("invalid sidecar admitted")
			}
		})
	}
	for _, ref := range []string{"missing", "assets/a.pdf/", "../assets/a.pdf/id", "assets/../a.pdf/id", "assets/a\\b.pdf/id"} {
		if _, _, err := annotationReference(ref); err == nil {
			t.Fatal(ref)
		}
	}
}
func TestMarkdownExportAnnotationSourceFailuresAndLinks(t *testing.T) {
	for _, kind := range []string{"asset-missing", "sidecar-missing", "asset-link", "sidecar-link", "sidecar-directory", "permission"} {
		t.Run(kind, func(t *testing.T) {
			if kind == "permission" && os.Geteuid() == 0 {
				t.Skip("root bypasses mode restriction")
			}
			m, _ := markdownFixture(t)
			asset := "assets/failure.pdf"
			file := filepath.Join(util.DataDir, asset)
			side := file + ".sya"
			annotationAsset(t, m, asset, `{"a":{"pages":[{"index":0}]}}`)
			outside := archiveWrite(t, t.TempDir(), "outside", []byte("outside"))
			switch kind {
			case "asset-missing":
				os.Remove(file)
			case "sidecar-missing":
				os.Remove(side)
			case "asset-link":
				os.Remove(file)
				os.Symlink(outside, file)
			case "sidecar-link":
				os.Remove(side)
				os.Symlink(outside, side)
			case "sidecar-directory":
				os.Remove(side)
				os.Mkdir(side, 0700)
			case "permission":
				os.Chmod(side, 0)
				t.Cleanup(func() { os.Chmod(side, 0600) })
			}
			if _, _, err := m.readAnnotationPage(asset + "/a"); err == nil {
				t.Fatal("bad source admitted")
			}
			raw, _ := os.ReadFile(outside)
			if string(raw) != "outside" {
				t.Fatal("outside changed")
			}
		})
	}
}
func TestMarkdownExportAnnotationCanceledOrSwitchedCachedRead(t *testing.T) {
	for _, kind := range []string{"cancel", "data", "temp"} {
		t.Run(kind, func(t *testing.T) {
			m, _ := markdownFixture(t)
			asset := "assets/cached.pdf"
			annotationAsset(t, m, asset, `{"a":{"pages":[{"index":0}]}}`)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			m.read.binding.scope.ctx = ctx
			if _, _, err := m.readAnnotationPage(asset + "/a"); err != nil {
				t.Fatal(err)
			}
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
			_, _, err := m.readAnnotationPage(asset + "/a")
			if err == nil {
				t.Fatal("cached read bypassed guard")
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}
func TestMarkdownExportAnnotationRenderedSnapshotAndReadonly(t *testing.T) {
	m, box := markdownFixture(t)
	asset := "assets/Report-" + ast.NewNodeID() + ".pdf"
	annotationAsset(t, m, asset, `{"a":{"pages":[{"index":2}]}}`)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj", map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeParagraph", "Children": []interface{}{annotationNode(asset, "a")}})
	file := filepath.Join(util.DataDir, box, id+".docj")
	before, _ := os.ReadFile(file)
	sideBefore, _ := os.ReadFile(filepath.Join(util.DataDir, asset+".sya"))
	m.settings.FileAnnotationRefMode = 0
	Conf.m.Lock()
	Conf.Export.FileAnnotationRefMode = 1
	Conf.m.Unlock()
	selected, err := m.selectDocuments([]string{id})
	if err != nil {
		t.Fatal(err)
	}
	defs, trees, _, err := m.prepare(selected)
	if err != nil {
		t.Fatal(err)
	}
	_, md, _, err := exportMarkdownContent(id, ".md", 2, defs, false, trees, m)
	if err != nil || !strings.Contains(md, "Report.pdf - p3 - Quoted text") || !strings.Contains(md, "#page=3") {
		t.Fatal(md, err)
	}
	after, _ := os.ReadFile(file)
	sideAfter, _ := os.ReadFile(filepath.Join(util.DataDir, asset+".sya"))
	if !bytes.Equal(before, after) || !bytes.Equal(sideBefore, sideAfter) {
		t.Fatal("source modified")
	}
}
func TestMarkdownExportAnnotationZIPIncludesFragmentAsset(t *testing.T) {
	m, box := markdownFixture(t)
	asset := "assets/Report space-" + ast.NewNodeID() + ".pdf"
	annotationAsset(t, m, asset, `{"a":{"pages":[{"index":1}]}}`)
	id := ast.NewNodeID()
	markdownDoc(t, m, box, "/"+id+".docj", map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeParagraph", "Children": []interface{}{annotationNode(asset, "a")}})
	_, url := ExportPandocConvertZip([]string{id}, "", ".md", context.Background())
	if url == "" {
		t.Fatal("export failed")
	}
	files, _ := os.ReadDir(filepath.Join(m.read.temp, "export"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	got := readOutputZip(t, filepath.Join(m.read.temp, "export", files[0].Name()))
	if string(got[asset]) != "PDF bytes" || !strings.Contains(string(got["Prepared Title.md"]), "#page=2") {
		t.Fatal(got)
	}
}
func TestMarkdownExportAnnotationFailureSuppressesZIP(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(map[bool]string{false: "broken-sidecar", true: "bad-ref-id"}[malformed], func(t *testing.T) {
			m, box := markdownFixture(t)
			asset := "assets/bad.pdf"
			annotationAsset(t, m, asset, `{"a":{"pages":[]}}`)
			node := annotationNode(asset, "a")
			if malformed {
				node["TextMarkFileAnnotationRefID"] = "missing"
			}
			id := ast.NewNodeID()
			markdownDoc(t, m, box, "/"+id+".docj", map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeParagraph", "Children": []interface{}{node}})
			old := archiveWrite(t, m.read.temp, "export/Prepared Title.md.zip", []byte("prior"))
			_, url := ExportPandocConvertZip([]string{id}, "", ".md", context.Background())
			if url != "" {
				t.Fatal(url)
			}
			raw, _ := os.ReadFile(old)
			if string(raw) != "prior" {
				t.Fatal("prior changed")
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
		})
	}
}
