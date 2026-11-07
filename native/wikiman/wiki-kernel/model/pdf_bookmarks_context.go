// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/pdfdevice"
)

type pdfExportHeading struct {
	id    string
	level int
}
type pdfPostprocessDocument struct {
	m        *markdownExportContext
	tree     *parse.Tree
	headings []pdfExportHeading
	assets   []string
}

// Original ProcessPDF loads the entire owning document even for a heading ID.
// Do not substitute the focused range used by HTML/PDF pre-rendering here.
func (m *markdownExportContext) preparePDFPostprocessDocument(id string, merge bool) (*pdfPostprocessDocument, error) {
	p := m.referenceRenderer()
	bt, err := p.lookup(id)
	if err != nil {
		return nil, err
	}
	if bt == nil {
		return nil, ErrBlockNotFound
	}
	tree, _, _, err := m.prepareHTMLDocument(bt.RootID, merge)
	if err != nil {
		return nil, err
	}
	headings, err := collectPDFHeadings(tree, m.guard)
	if err != nil {
		return nil, err
	}
	assets, err := getAssetsLinkDestsWithAV(tree.Root, m.read.readExportView)
	if err != nil {
		return nil, err
	}
	if err = m.checkExportRenderTree(tree); err != nil {
		return nil, err
	}
	return &pdfPostprocessDocument{m: m, tree: tree, headings: headings, assets: assets}, nil
}
func collectPDFHeadings(tree *parse.Tree, guard func() error) (headings []pdfExportHeading, err error) {
	if tree == nil || tree.Root == nil || guard == nil {
		return nil, errors.New("pdf_headings_dependencies_unbound")
	}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = guard(); err != nil {
			return ast.WalkStop
		}
		if enter && n.Type == ast.NodeHeading && !n.ParentIs(ast.NodeBlockquote) {
			headings = append(headings, pdfExportHeading{n.ID, n.HeadingLevel})
			return ast.WalkSkipChildren
		}
		return ast.WalkContinue
	})
	if err != nil {
		return nil, err
	}
	if err = guard(); err != nil {
		return nil, err
	}
	return headings, nil
}
func (d *pdfPostprocessDocument) guard() error {
	if d == nil || d.m == nil {
		return errors.New("pdf_document_unbound")
	}
	return d.m.checkExportRenderTree(d.tree)
}
func (d *pdfPostprocessDocument) bookmarks(pdf *pdfmodel.Context) error {
	if err := d.guard(); err != nil {
		return err
	}
	r, ctx := d.m.read.binding.scope.commit.r, d.m.read.binding.scope.ctx
	lookup := func(id string) (string, bool, error) {
		if err := d.guard(); err != nil {
			return "", false, err
		}
		row, ok, err := (indexdb.CachedBlocks{DB: r.main.DB}).Get(ctx, id)
		if err == nil {
			err = d.guard()
		}
		return row.Content, ok, err
	}
	return applyPDFBookmarks(pdf, d.headings, lookup, d.guard)
}
func deviceHeadings(headings []pdfExportHeading) []pdfdevice.Heading {
	out := make([]pdfdevice.Heading, 0, len(headings))
	for _, h := range headings {
		out = append(out, pdfdevice.Heading{ID: h.id, Level: h.level})
	}
	return out
}
func readPDFOutlineLinks(pdf *pdfmodel.Context, guard func() error) ([]pdfmodel.LinkAnnotation, error) {
	return pdfdevice.ReadOutlineLinks(pdf, guard)
}
func buildPDFBookmarks(links []pdfmodel.LinkAnnotation, headings []pdfExportHeading, lookup func(string) (string, bool, error), guard func() error) ([]*pdfcpu.Bookmark, error) {
	return pdfdevice.BuildBookmarks(links, deviceHeadings(headings), lookup, guard)
}
func applyPDFBookmarks(pdf *pdfmodel.Context, headings []pdfExportHeading, lookup func(string) (string, bool, error), guard func() error) error {
	return pdfdevice.ApplyBookmarks(pdf, deviceHeadings(headings), lookup, guard)
}
