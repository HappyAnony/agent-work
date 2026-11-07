//go:build fts5

// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/util"
)

const pdfAssetOrigin = "http://127.0.0.1:60000"

func pdfAssetsFixture(t *testing.T) (*pdfPostprocessDocument, map[string][]byte) {
	t.Helper()
	m, id, before := fileContextFixture(t)
	m.htmlLocalAnchorPrefix = pdfAssetOrigin + "/#"
	d, e := m.preparePDFPostprocessDocument(id, false)
	if e != nil {
		t.Fatal(e)
	}
	return d, before
}
func pdfAssetTestPDF(t *testing.T, uris ...string) *pdfmodel.Context {
	t.Helper()
	p := pdfTestContext(t)
	if _, e := pdfcpu.RemoveAnnotations(p, nil, nil, nil, false); e != nil {
		t.Fatal(e)
	}
	annotations := []pdfmodel.AnnotationRenderer{}
	for _, uri := range uris {
		annotations = append(annotations, pdfmodel.NewLinkAnnotation(*types.RectForWidthAndHeight(10, 40, 80, 20), "", "", "", 0, nil, nil, uri, nil, false, 0, 0))
	}
	annotations = append(annotations, pdfmodel.NewTextAnnotation(*types.RectForWidthAndHeight(100, 100, 10, 10), "retained note", "note", "", 0, nil, "", nil, nil, "", "", 0, 0, 0, false, "Note"))
	if _, e := pdfcpu.AddAnnotationsMap(p, map[int][]pdfmodel.AnnotationRenderer{1: annotations}, false); e != nil {
		t.Fatal(e)
	}
	return pdfBookmarkRoundTrip(t, p)
}
func pdfAssetLinks(t *testing.T, p *pdfmodel.Context) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	_, links, e := readPDFAssetLinks(p, func(string) bool { return false }, func() error { return nil })
	if e != nil {
		t.Fatal(e)
	}
	for _, l := range links {
		out[l.URI] = true
	}
	return out
}
func pdfEmbeddedBytes(t *testing.T, p *pdfmodel.Context) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	page, e := p.PageDictIndRef(1)
	if e != nil {
		t.Fatal(e)
	}
	dict, e := p.DereferenceDict(*page)
	if e != nil {
		t.Fatal(e)
	}
	array, e := p.DereferenceArray(dict["Annots"])
	if e != nil {
		t.Fatal(e)
	}
	for _, item := range array {
		ann, e := p.DereferenceDict(item)
		if e != nil {
			t.Fatal(e)
		}
		if subtype := ann.NameEntry("Subtype"); subtype == nil || *subtype != "FileAttachment" {
			continue
		}
		spec, e := p.DereferenceDict(ann["FS"])
		if e != nil {
			t.Fatal(e)
		}
		ef, e := p.DereferenceDict(spec["EF"])
		if e != nil {
			t.Fatal(e)
		}
		stream, _, e := p.DereferenceStreamDict(ef["F"])
		if e != nil {
			t.Fatal(e)
		}
		if e = stream.Decode(); e != nil {
			t.Fatal(e)
		}
		filename, e := p.DereferenceText(spec["UF"])
		if e != nil {
			t.Fatal(e)
		}
		out[filename] = append([]byte(nil), stream.Content...)
	}
	return out
}
func TestPDFAssetsOwnedKeepAndEmbedRoundTrip(t *testing.T) {
	for _, embed := range []bool{false, true} {
		t.Run(map[bool]string{false: "relative-links", true: "embedded"}[embed], func(t *testing.T) {
			d, before := pdfAssetsFixture(t)
			external := "https://example.invalid/a%2Fb?q=%2520"
			p := pdfAssetTestPDF(t, pdfAssetOrigin+"/export/temp/assets/same.pdf?download=1#page=2", external)
			if e := d.assetLinks(p, embed); e != nil {
				t.Fatal(e)
			}
			p = pdfBookmarkRoundTrip(t, p)
			links := pdfAssetLinks(t, p)
			if links[pdfAssetOrigin+"/export/temp/assets/same.pdf?download=1#page=2"] {
				t.Fatal("old asset link retained")
			}
			if !links[external] || len(p.PageAnnots[1][pdfmodel.AnnText].Map) != 1 {
				t.Fatal("external link or note lost", links)
			}
			files := pdfEmbeddedBytes(t, p)
			if embed {
				if string(files["same.pdf"]) != "global PDF" || links["assets/same.pdf#page=2"] {
					t.Fatal(files, links)
				}
			} else if !links["assets/same.pdf#page=2"] || len(files) != 0 {
				t.Fatal(links, files)
			}
			assertFileSourceUnchanged(t, before)
			if d.m.read.output != nil {
				t.Fatal("asset core published files")
			}
		})
	}
}
func TestPDFAssetsEncodedNamesAndCapturedOrigin(t *testing.T) {
	m, box := markdownFixture(t)
	m.htmlLocalAnchorPrefix = pdfAssetOrigin + "/#"
	name := "中文 # %.txt"
	relative := "assets/" + name
	rawURI := "assets/" + url.PathEscape(name)
	filename := archiveWrite(t, m.read.binding.scope.commit.r.data, relative, []byte("encoded payload"))
	id := ast.NewNodeID()
	nativePreRenderDoc(t, m, box, "/"+id+".docj", footParagraph(ast.NewNodeID(), resourceLink(rawURI)))
	d, e := m.preparePDFPostprocessDocument(id, false)
	if e != nil {
		t.Fatal(e)
	}
	old := util.ServerPort
	util.ServerPort = "changed"
	t.Cleanup(func() { util.ServerPort = old })
	p := pdfAssetTestPDF(t, pdfAssetOrigin+"/"+rawURI, "http://127.0.0.1:60001/assets/not-owned.txt")
	if e = d.assetLinks(p, true); e != nil {
		t.Fatal(e)
	}
	p = pdfBookmarkRoundTrip(t, p)
	files := pdfEmbeddedBytes(t, p)
	if string(files[name]) != "encoded payload" {
		t.Fatal(files)
	}
	if !pdfAssetLinks(t, p)["http://127.0.0.1:60001/assets/not-owned.txt"] {
		t.Fatal("foreign origin touched")
	}
	legacy := pdfAssetTestPDF(t, pdfAssetOrigin+"/"+rawURI)
	util.ServerPort = "60000"
	processPDFLinkEmbedAssets(legacy, d.assets, true)
	if got := pdfEmbeddedBytes(t, pdfBookmarkRoundTrip(t, legacy)); string(got[name]) != "encoded payload" {
		t.Fatal("legacy encoded destination skipped", got)
	}
	b, _ := os.ReadFile(filename)
	if string(b) != "encoded payload" {
		t.Fatal("source changed")
	}
}
func TestPDFAssetsRejectsUnselectedTraversalAndSourceLinks(t *testing.T) {
	for _, kind := range []string{"unselected", "traversal", "source-link", "pipe", "cancel", "closed"} {
		t.Run(kind, func(t *testing.T) {
			d, before := pdfAssetsFixture(t)
			uri := pdfAssetOrigin + "/assets/same.pdf"
			switch kind {
			case "unselected":
				uri = pdfAssetOrigin + "/assets/secret.txt"
				archiveWrite(t, d.m.read.binding.scope.commit.r.data, "assets/secret.txt", []byte("private"))
			case "traversal":
				uri = pdfAssetOrigin + "/assets/%2e%2e/private"
			case "source-link":
				file := filepath.Join(d.m.read.binding.scope.commit.r.data, "assets/same.pdf")
				os.Remove(file)
				os.Symlink(filepath.Join(t.TempDir(), "foreign"), file)
			case "pipe":
				file := filepath.Join(d.m.read.binding.scope.commit.r.data, "assets/same.pdf")
				os.Remove(file)
				if e := exec.Command("mkfifo", file).Run(); e != nil {
					t.Skip("mkfifo unavailable")
				}
			case "cancel":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.m.read.binding.scope.ctx = ctx
			case "closed":
				owner := d.m.read.binding.scope.commit.r.owner
				owner.repoMu.Lock()
				owner.repoClosed = true
				owner.repoMu.Unlock()
				t.Cleanup(func() { owner.repoMu.Lock(); owner.repoClosed = false; owner.repoMu.Unlock() })
			}
			p := pdfAssetTestPDF(t, uri)
			if e := d.assetLinks(p, true); e == nil {
				t.Fatal("unsafe source accepted", kind)
			}
			if kind != "source-link" && kind != "pipe" {
				assertFileSourceUnchanged(t, before)
			}
		})
	}
}
func TestPDFAssetsMissingSourcesAndLegacyCoreCompatibility(t *testing.T) {
	for _, embed := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep", true: "embed"}[embed], func(t *testing.T) {
			d, before := pdfAssetsFixture(t)
			uri := pdfAssetOrigin + "/assets/same.pdf"
			owned, legacy := pdfAssetTestPDF(t, uri), pdfAssetTestPDF(t, uri)
			if e := d.assetLinks(owned, embed); e != nil {
				t.Fatal(e)
			}
			old := util.ServerPort
			util.ServerPort = "60000"
			t.Cleanup(func() { util.ServerPort = old })
			processPDFLinkEmbedAssets(legacy, d.assets, embed)
			owned = pdfBookmarkRoundTrip(t, owned)
			legacy = pdfBookmarkRoundTrip(t, legacy)
			if embed {
				if !bytes.Equal(pdfEmbeddedBytes(t, owned)["same.pdf"], pdfEmbeddedBytes(t, legacy)["same.pdf"]) {
					t.Fatal("legacy embed bytes differ")
				}
			} else if !pdfAssetLinks(t, owned)["assets/same.pdf"] || !pdfAssetLinks(t, legacy)["assets/same.pdf"] {
				t.Fatal("legacy relative links differ")
			}
			assertFileSourceUnchanged(t, before)
		})
	}
	d, _ := pdfAssetsFixture(t)
	d.assets = append(d.assets, "assets/missing.txt")
	p := pdfAssetTestPDF(t, pdfAssetOrigin+"/assets/missing.txt")
	if e := d.assetLinks(p, true); e != nil {
		t.Fatal(e)
	}
	p = pdfBookmarkRoundTrip(t, p)
	if len(pdfEmbeddedBytes(t, p)) != 0 {
		t.Fatal("missing source embedded")
	}
}
func TestPDFAssetsReaderLimitsAndCancellation(t *testing.T) {
	total := int64(8)
	r := &pdfAssetReader{reader: strings.NewReader("123456789"), guard: func() error { return nil }, remaining: 8, total: &total}
	if _, e := io.ReadAll(r); e == nil {
		t.Fatal("read limit ignored")
	}
	total = 100
	r = &pdfAssetReader{reader: strings.NewReader("payload"), guard: func() error { return context.Canceled }, remaining: 100, total: &total}
	if _, e := io.ReadAll(r); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	total = 3
	r = &pdfAssetReader{reader: strings.NewReader("long"), guard: func() error { return nil }, remaining: 100, total: &total}
	if _, e := io.ReadAll(r); e == nil {
		t.Fatal("aggregate budget ignored")
	}
}
func TestPDFAssetsOversizedFileRejectedBeforeReading(t *testing.T) {
	m, box := markdownFixture(t)
	file := filepath.Join(m.read.binding.scope.commit.r.data, "assets/oversized.bin")
	if e := os.MkdirAll(filepath.Dir(file), 0755); e != nil {
		t.Fatal(e)
	}
	f, e := os.Create(file)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.Truncate(pdfAssetFileLimit + 1); e != nil {
		t.Fatal(e)
	}
	f.Close()
	id := ast.NewNodeID()
	nativePreRenderDoc(t, m, box, "/"+id+".docj", footParagraph(ast.NewNodeID(), resourceLink("assets/oversized.bin")))
	m.htmlLocalAnchorPrefix = pdfAssetOrigin + "/#"
	d, e := m.preparePDFPostprocessDocument(id, false)
	if e != nil {
		t.Fatal(e)
	}
	p := pdfAssetTestPDF(t, pdfAssetOrigin+"/assets/oversized.bin")
	if e = d.assetLinks(p, true); e == nil || !strings.Contains(e.Error(), "pdf_asset_read_limit") {
		t.Fatal(e)
	}
	if info, e := os.Stat(file); e != nil || info.Size() != pdfAssetFileLimit+1 {
		t.Fatal("oversized source altered", e)
	}
}
func TestPDFAssetsMalformedScansAndURLBoundaries(t *testing.T) {
	for _, uri := range []string{pdfAssetOrigin + "/assets/%2e%2e/private", pdfAssetOrigin + "/assets/a%5Cb", pdfAssetOrigin + "/assets/%ZZ", "https://example.invalid/assets/a"} {
		if _, e := normalizePDFAssetURL(uri, pdfAssetOrigin); e == nil {
			t.Fatal("invalid URL accepted", uri)
		}
	}
	for _, uri := range []string{pdfAssetOrigin + "/assets/a%2525.txt", pdfAssetOrigin + "/export/temp/assets/中文.txt?x=1#p"} {
		if u, e := normalizePDFAssetURL(uri, pdfAssetOrigin); e != nil || u.path == "" {
			t.Fatal(u, e)
		}
	}
	if a, b, e := PdfListLinks(nil); e == nil || a != nil || b != nil {
		t.Fatal("nil scan accepted")
	}
	p := pdfTestContext(t)
	p.PageAnnots = map[int]pdfmodel.PgAnnots{1: {pdfmodel.AnnLink: pdfmodel.Annot{Map: pdfmodel.AnnotMap{1: pdfmodel.Annotation{SubType: pdfmodel.AnnLink}}}}}
	if a, b, e := PdfListLinks(p); e == nil || a != nil || b != nil {
		t.Fatal("malformed annotation accepted")
	}
	if e := (&pdfPostprocessDocument{}).assetLinks(p, true); e == nil {
		t.Fatal("unbound source accepted")
	}
}
