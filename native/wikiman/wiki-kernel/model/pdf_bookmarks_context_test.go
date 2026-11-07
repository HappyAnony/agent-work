//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"os"
	"reflect"
	"testing"
	"wiki-kernel/av"
)

func pdfBookmarkLink(id string, page int, y float64) pdfmodel.LinkAnnotation {
	return pdfmodel.LinkAnnotation{Annotation: pdfmodel.Annotation{Rect: *types.RectForWidthAndHeight(10, y-10, 40, 10)}, URI: PdfOutlineScheme + "://" + id, Page: page}
}
func pdfTestContext(t *testing.T) *pdfmodel.Context {
	t.Helper()
	raw, e := os.ReadFile("../testdata/parsertest.pdf")
	if e != nil {
		t.Fatal(e)
	}
	conf := &pdfmodel.Configuration{Reader15: true, ValidationMode: pdfmodel.ValidationRelaxed, Eol: types.EolLF}
	pdf, e := api.ReadContext(bytes.NewReader(raw), conf)
	if e != nil {
		t.Fatal(e)
	}
	if e = api.ValidateContext(pdf); e != nil {
		t.Fatal(e)
	}
	return pdf
}
func installTestOutlineLinks(pdf *pdfmodel.Context, links ...pdfmodel.LinkAnnotation) {
	pdf.PageAnnots = map[int]pdfmodel.PgAnnots{}
	for i, l := range links {
		if pdf.PageAnnots[l.Page] == nil {
			pdf.PageAnnots[l.Page] = pdfmodel.PgAnnots{pdfmodel.AnnLink: pdfmodel.Annot{Map: pdfmodel.AnnotMap{}}}
		}
		pdf.PageAnnots[l.Page][pdfmodel.AnnLink].Map[i+1] = l
	}
}
func pdfBookmarkRoundTrip(t *testing.T, pdf *pdfmodel.Context) *pdfmodel.Context {
	t.Helper()
	var output bytes.Buffer
	if e := api.WriteContext(pdf, &output); e != nil {
		t.Fatal(e)
	}
	if !bytes.HasPrefix(output.Bytes(), []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	next, e := api.ReadContext(bytes.NewReader(output.Bytes()), &pdfmodel.Configuration{Reader15: true, ValidationMode: pdfmodel.ValidationRelaxed, Eol: types.EolLF})
	if e != nil {
		t.Fatal(e)
	}
	if e = api.ValidateContext(next); e != nil {
		t.Fatal(e)
	}
	return next
}
func TestPDFBookmarksOriginalHierarchyDecodingAndMissingLinks(t *testing.T) {
	links := []pdfmodel.LinkAnnotation{pdfBookmarkLink("a", 2, 200), pdfBookmarkLink("b", 3, 180), pdfBookmarkLink("c", 4, 160), pdfBookmarkLink("d", 5, 140), pdfBookmarkLink("absent", 6, 120)}
	headings := []pdfExportHeading{{"a", 1}, {"no-link", 2}, {"b", 3}, {"c", 1}, {"d", 2}, {"absent", 3}}
	lookup := func(id string) (string, bool, error) {
		if id == "absent" {
			return "", false, nil
		}
		if id == "a" || id == "b" {
			return "Shared+title%21", true, nil
		}
		return id, true, nil
	}
	got, e := buildPDFBookmarks(links, headings, lookup, func() error { return nil })
	if e != nil || len(got) != 2 || got[0].Title != "Shared title!" || len(got[0].Kids) != 1 || got[0].Kids[0].Title != "Shared title!\x01" || got[0].Kids[0].Parent != got[0] || len(got[1].Kids) != 1 || got[1].Kids[0].Title != "d" {
		t.Fatal(got, e)
	}
	if got[0].AbsPos != 200 || got[0].PageFrom != 2 || got[0].Kids[0].Level != 3 {
		t.Fatal("position or original heading level changed")
	}
	if links[0].Page != 2 {
		t.Fatal("caller links modified")
	}
}
func TestPDFPostprocessPreparationUsesOwningDocumentAndMerge(t *testing.T) {
	m, box := markdownFixture(t)
	id, a, b, quoted := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	nativePreRenderDoc(t, m, box, "/"+id+".docj", focusHeading(a, 2, "Before selection"), footParagraph(ast.NewNodeID(), resourceLink("assets/paper.pdf")), focusHeading(b, 2, "Selected"), map[string]interface{}{"ID": ast.NewNodeID(), "Type": "NodeBlockquote", "Children": []interface{}{map[string]interface{}{"Type": "NodeBlockquoteMarker", "Data": ">"}, focusHeading(quoted, 3, "Quoted")}})
	d, e := m.preparePDFPostprocessDocument(b, false)
	if e != nil || d.tree.ID != id || len(d.headings) != 2 || d.headings[0].id != a || d.headings[1].id != b || len(d.assets) != 1 || d.assets[0] != "assets/paper.pdf" {
		t.Fatal(d, e)
	}
	m2, _, before := mergeFixture(t)
	merged, e := m2.preparePDFPostprocessDocument(mergeRoot, true)
	if e != nil {
		t.Fatal(e)
	}
	seen := map[string]bool{}
	for _, h := range merged.headings {
		seen[h.id] = true
	}
	if !seen[mergeA] || !seen[mergeGrand] {
		t.Fatal("merged document headings missing", seen)
	}
	assertFileSourceUnchanged(t, before)
	if m2.read.output != nil {
		t.Fatal("preparation published output")
	}
}
func TestPDFBookmarksOwnedSQLRealPDFRoundTripAndLegacyWrapper(t *testing.T) {
	m, box := markdownFixture(t)
	id, a, b := ast.NewNodeID(), ast.NewNodeID(), ast.NewNodeID()
	nativePreRenderDoc(t, m, box, "/"+id+".docj", focusHeading(a, 1, "A"), focusHeading(b, 2, "B"))
	insertMarkdownRefText(t, m, a, "Encoded+title%21")
	insertMarkdownRefText(t, m, b, "Second")
	d, e := m.preparePDFPostprocessDocument(id, false)
	if e != nil {
		t.Fatal(e)
	}
	links := []pdfmodel.LinkAnnotation{pdfBookmarkLink(a, 1, 200), pdfBookmarkLink(b, 1, 180)}
	first, legacy := pdfTestContext(t), pdfTestContext(t)
	installTestOutlineLinks(first, links...)
	installTestOutlineLinks(legacy, links...)
	if e = d.bookmarks(first); e != nil {
		t.Fatal(e)
	}
	processPDFBookmarks(legacy, []*ast.Node{{ID: a, HeadingLevel: 1}, {ID: b, HeadingLevel: 2}})
	// pdfcpu's in-memory Outlines cache is refreshed by validation/readback,
	// so compare the serialized artifacts rather than that stale cache.
	first = pdfBookmarkRoundTrip(t, first)
	legacy = pdfBookmarkRoundTrip(t, legacy)
	left, e := pdfcpu.Bookmarks(first)
	if e != nil {
		t.Fatal(e)
	}
	right, e := pdfcpu.Bookmarks(legacy)
	if e != nil {
		t.Fatal(e)
	}
	if len(left) != 1 || len(right) != 1 || left[0].Title != "Encoded title!" || left[0].Title != right[0].Title || len(left[0].Kids) != 1 || left[0].Kids[0].Title != right[0].Kids[0].Title {
		t.Fatal("shared legacy/owned PDF projection", left, right)
	}
	var output bytes.Buffer
	if e = api.WriteContext(first, &output); e != nil {
		t.Fatal(e)
	}
	if !bytes.HasPrefix(output.Bytes(), []byte("%PDF-")) {
		t.Fatal("invalid PDF")
	}
	again, e := api.ReadContext(bytes.NewReader(output.Bytes()), &pdfmodel.Configuration{Reader15: true, ValidationMode: pdfmodel.ValidationRelaxed, Eol: types.EolLF})
	if e != nil {
		t.Fatal(e)
	}
	if e = api.ValidateContext(again); e != nil {
		t.Fatal(e)
	}
	restored, e := pdfcpu.Bookmarks(again)
	if e != nil || len(restored) != 1 || len(restored[0].Kids) != 1 || restored[0].Kids[0].Title != "Second" {
		t.Fatal(restored, e)
	}
	if m.read.output != nil {
		t.Fatal("bookmark port wrote publication")
	}
}
func TestPDFBookmarksFailuresReturnNoPartialPlan(t *testing.T) {
	links := []pdfmodel.LinkAnnotation{pdfBookmarkLink("a", 1, 200), pdfBookmarkLink("b", 2, 180)}
	headings := []pdfExportHeading{{"a", 1}, {"b", 2}}
	for _, kind := range []string{"lookup", "cancel", "invalid-heading"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			guard := func() error {
				calls++
				if kind == "cancel" && calls > 2 {
					return context.Canceled
				}
				return nil
			}
			lookup := func(id string) (string, bool, error) {
				if kind == "lookup" && id == "b" {
					return "", false, errors.New("synthetic query")
				}
				return id, true, nil
			}
			h := headings
			if kind == "invalid-heading" {
				h = []pdfExportHeading{{"a", 1}, {"a", 2}}
			}
			got, e := buildPDFBookmarks(links, h, lookup, guard)
			if e == nil || got != nil {
				t.Fatal("partial bookmarks returned", got, e)
			}
		})
	}
	m, box := markdownFixture(t)
	id, a := ast.NewNodeID(), ast.NewNodeID()
	nativePreRenderDoc(t, m, box, "/"+id+".docj", focusHeading(a, 1, "A"))
	d, e := m.preparePDFPostprocessDocument(id, false)
	if e != nil {
		t.Fatal(e)
	}
	pdf := pdfTestContext(t)
	installTestOutlineLinks(pdf, pdfBookmarkLink(a, 1, 200))
	m.read.binding.scope.commit.r.main.DB.Close()
	if e = d.bookmarks(pdf); e == nil {
		t.Fatal("SQL failure hidden")
	}
	if e := (&pdfPostprocessDocument{}).guard(); e == nil {
		t.Fatal("unbound PDF document accepted")
	}
}
func TestPDFBookmarksScannerGuardsAndMalformedAnnotation(t *testing.T) {
	pdf := pdfTestContext(t)
	installTestOutlineLinks(pdf, pdfBookmarkLink("a", 1, 200))
	links, e := PdfListToCLinks(pdf)
	if e != nil || len(links) != 1 || links[0].Page != 1 {
		t.Fatal(links, e)
	}
	if got, e := readPDFOutlineLinks(pdf, func() error { return context.Canceled }); !errors.Is(e, context.Canceled) || got != nil {
		t.Fatal(got, e)
	}
	pdf.PageAnnots[1][pdfmodel.AnnLink].Map[2] = pdfmodel.Annotation{SubType: pdfmodel.AnnLink}
	if got, e := PdfListToCLinks(pdf); e == nil || got != nil {
		t.Fatal("bad annotation accepted", got, e)
	}
	if got, e := PdfListToCLinks(nil); e == nil || got != nil {
		t.Fatal("nil context accepted")
	}
}
func TestPDFPostprocessAVDependenciesAndSourceRemainReadOnly(t *testing.T) {
	for _, layout := range []av.LayoutType{av.LayoutTypeTable, av.LayoutTypeGallery, av.LayoutTypeKanban} {
		t.Run(string(layout), func(t *testing.T) {
			m, _, tree, file := markdownAVFixture(t, layout)
			before, _ := os.ReadFile(file)
			d, e := m.preparePDFPostprocessDocument(tree.ID, false)
			if e != nil || d.tree == nil {
				t.Fatal(e)
			}
			after, _ := os.ReadFile(file)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("AV source mutated")
			}
			if m.read.output != nil {
				t.Fatal("output capability used")
			}
		})
	}
	m, box := markdownFixture(t)
	id := ast.NewNodeID()
	nativePreRenderDoc(t, m, box, "/"+id+".docj", focusHeading(ast.NewNodeID(), 1, "A"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.read.binding.scope.ctx = ctx
	if d, e := m.preparePDFPostprocessDocument(id, false); e == nil || d != nil {
		t.Fatal("canceled preparation")
	}
}
