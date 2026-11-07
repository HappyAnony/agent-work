// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"path"
	"strings"
	"wiki-kernel/util"
)

type ExportHTMLPreviewOptions struct{ Image, KeepFold, Merge bool }
type ExportHTMLPreviewResult struct {
	Name, Content, Type string
	Attributes          map[string]string
}

func ExportPreviewHTMLContext(ctx context.Context, id string, options ExportHTMLPreviewOptions) (ExportHTMLPreviewResult, error) {
	m, err := captureMarkdownExport(ctx)
	if err != nil {
		return ExportHTMLPreviewResult{}, err
	}
	return m.previewExportHTML(id, options)
}
func (m *markdownExportContext) previewExportHTML(id string, options ExportHTMLPreviewOptions) (ExportHTMLPreviewResult, error) {
	// Both image and PDF requests retain the original PDF-style pre-render DOM.
	_, node, name, content, err := m.renderHTMLDocument(id, true, options.KeepFold, options.Merge)
	if err != nil {
		return ExportHTMLPreviewResult{}, err
	}
	result := ExportHTMLPreviewResult{Name: name, Content: strings.ReplaceAll(content, m.htmlLocalAnchorPrefix, "#"), Attributes: map[string]string{}}
	if node != nil {
		result.Attributes = parse.IAL2Map(node.KramdownIAL)
		result.Type = node.Type.String()
	}
	if err = m.guard(); err != nil {
		return ExportHTMLPreviewResult{}, err
	}
	return result, nil
}
func (m *markdownExportContext) renderHTMLDocument(id string, pdf, keepFold, merge bool) (tree *parse.Tree, node *ast.Node, name, content string, err error) {
	tree, node, bt, err := m.prepareHTMLDocument(id, merge)
	if err != nil {
		return nil, nil, "", "", err
	}
	failed := func(e error) (*parse.Tree, *ast.Node, string, string, error) { return nil, nil, "", "", e }
	if pdf {
		if err = addExportPDFOutline(tree, m.guard); err != nil {
			return failed(err)
		}
	}
	p := m.referenceRenderer()
	tree = exportTree(tree, true, keepFold, true, m.settings.BlockRefMode, m.settings.BlockEmbedMode, m.settings.FileAnnotationRefMode, m.settings.TagOpenMarker, m.settings.TagCloseMarker, m.settings.BlockRefTextLeft, m.settings.BlockRefTextRight, m.settings.AddTitle, m.settings.InlineMemo, true, true, map[string]*parse.Tree{}, p)
	if err = p.check(); err != nil {
		return failed(err)
	}
	adjustExportHeading(bt, tree, m.settings.AddTitle, []*exportReferenceRenderer{p})
	if err = p.check(); err != nil {
		return failed(err)
	}
	name = util.FilterFileName(path.Base(tree.HPath))
	if pdf {
		if err = processExportIFrames(tree, m.guard); err != nil {
			return failed(err)
		}
	}
	if err = m.checkExportRenderTree(tree); err != nil {
		return failed(err)
	}
	content, err = renderExportHTML(tree, p.parser(), m.guard)
	if err != nil {
		return failed(err)
	}
	return tree, node, name, content, nil
}
func addExportPDFOutline(tree *parse.Tree, guard func() error) (err error) {
	if tree == nil || tree.Root == nil || guard == nil {
		return errors.New("html_outline_dependencies_unbound")
	}
	var headings []*ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = guard(); err != nil {
			return ast.WalkStop
		}
		if enter && n.Type == ast.NodeHeading && !n.ParentIs(ast.NodeBlockquote) {
			headings = append(headings, n)
			return ast.WalkSkipChildren
		}
		return ast.WalkContinue
	})
	if err != nil {
		return err
	}
	for _, h := range headings {
		if err = guard(); err != nil {
			return err
		}
		link := &ast.Node{Type: ast.NodeLink}
		link.AppendChild(&ast.Node{Type: ast.NodeOpenBracket})
		link.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(" ")})
		link.AppendChild(&ast.Node{Type: ast.NodeCloseBracket})
		link.AppendChild(&ast.Node{Type: ast.NodeOpenParen})
		link.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte(PdfOutlineScheme + "://" + h.ID)})
		link.AppendChild(&ast.Node{Type: ast.NodeCloseParen})
		h.PrependChild(link)
	}
	return guard()
}
func processExportIFrames(tree *parse.Tree, guard func() error) (err error) {
	if tree == nil || tree.Root == nil || guard == nil {
		return errors.New("html_iframe_dependencies_unbound")
	}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = guard(); err != nil {
			return ast.WalkStop
		}
		if !enter || n.Type != ast.NodeIFrame {
			return ast.WalkContinue
		}
		index := bytes.Index(n.Tokens, []byte("src=\""))
		if index < 0 {
			n.Type = ast.NodeParagraph
			n.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: n.Tokens})
			return ast.WalkContinue
		}
		src := n.Tokens[index+len("src=\""):]
		end := bytes.IndexByte(src, '"')
		if end < 0 {
			err = errors.New("html_iframe_source_invalid")
			return ast.WalkStop
		}
		src = html.UnescapeHTML(src[:end])
		n.Type = ast.NodeParagraph
		link := &ast.Node{Type: ast.NodeLink}
		link.AppendChild(&ast.Node{Type: ast.NodeOpenBracket})
		link.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: src})
		link.AppendChild(&ast.Node{Type: ast.NodeCloseBracket})
		link.AppendChild(&ast.Node{Type: ast.NodeOpenParen})
		link.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: src})
		link.AppendChild(&ast.Node{Type: ast.NodeCloseParen})
		n.AppendChild(link)
		return ast.WalkContinue
	})
	if err != nil {
		return err
	}
	return guard()
}
func renderExportHTML(tree *parse.Tree, engine *lute.Lute, guard func() error) (string, error) {
	if tree == nil || tree.Root == nil || engine == nil || guard == nil {
		return "", errors.New("html_render_dependencies_unbound")
	}
	if err := guard(); err != nil {
		return "", err
	}
	engine.SetFootnotes(true)
	engine.RenderOptions.ProtyleContenteditable = false
	engine.SetProtyleMarkNetImg(false)
	engine.SetSanitize(false)
	content := string(render.NewProtyleExportRenderer(tree, engine.RenderOptions).Render())
	if err := guard(); err != nil {
		return "", err
	}
	return content, nil
}
