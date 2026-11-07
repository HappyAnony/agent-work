// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	"strings"
	"wiki-kernel/treenode"
)

func PreviewContext(ctx context.Context, id string, fillCSSVar bool) (string, error) {
	m, err := captureMarkdownExport(ctx)
	if err != nil {
		return "", err
	}
	return m.preview(id, fillCSSVar)
}
func (m *markdownExportContext) preview(id string, fillCSSVar bool) (string, error) {
	if err := m.guard(); err != nil {
		return "", err
	}
	tree, bt, err := m.prepareFocusedExport(id)
	if err != nil {
		return "", err
	}
	p := m.referenceRenderer()
	tree = exportTree(tree, false, false, true, m.settings.BlockRefMode, m.settings.BlockEmbedMode, m.settings.FileAnnotationRefMode, "#", "#", m.settings.BlockRefTextLeft, m.settings.BlockRefTextRight, m.settings.AddTitle, m.settings.InlineMemo, true, true, map[string]*parse.Tree{}, p)
	if err = p.check(); err != nil {
		return "", err
	}
	if err = m.checkExportRenderTree(tree); err != nil {
		return "", err
	}
	var theme func() (map[string]string, error)
	if fillCSSVar {
		theme = m.loadThemeStyles
	}
	return renderHTMLPreview(tree, bt, p.parser(), m.settings.AddTitle, theme, m.guard)
}

// Preserve the original preview pipeline. The caller admits the source candidate;
// CSS is applied only to the Markdown-derived local tree, not an arbitrary tree
// passed to the general source-owned theme port.
func renderHTMLPreview(tree *parse.Tree, bt *treenode.BlockTree, engine *lute.Lute, addTitle bool, theme func() (map[string]string, error), guard func() error) (html string, err error) {
	if tree == nil || tree.Root == nil || bt == nil || engine == nil || guard == nil {
		return "", errors.New("preview_dependencies_unbound")
	}
	if err = guard(); err != nil {
		return "", err
	}
	enableLuteInlineSyntax(engine)
	engine.SetFootnotes(true)
	addBlockIALNodes(tree, false)
	if tree.Root.FirstChild != nil {
		adjustHeadingLevelWithTitle(bt, tree, addTitle)
	}
	var unlinks []*ast.Node
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = guard(); err != nil {
			return ast.WalkStop
		}
		if enter && n.Type == ast.NodeKramdownBlockIAL && n.Previous != nil && n.Previous.Type == ast.NodeSuperBlock {
			unlinks = append(unlinks, n)
		}
		return ast.WalkContinue
	})
	if err != nil {
		return "", err
	}
	for _, n := range unlinks {
		n.Unlink()
	}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = guard(); err != nil {
			return ast.WalkStop
		}
		if !enter {
			return ast.WalkContinue
		}
		if n.Type == ast.NodeFootnotesRef && n.Next != nil {
			text := n.NextNodeText()
			if strings.HasPrefix(text, "(") && strings.HasSuffix(text, ")") {
				n.InsertAfter(&ast.Node{Type: ast.NodeText, Tokens: []byte(editor.Zwsp)})
			}
		}
		return ast.WalkContinue
	})
	if err != nil {
		return "", err
	}
	md, err := lute.FormatNodeSync(tree.Root, engine.ParseOptions, engine.RenderOptions)
	if err != nil {
		return "", err
	}
	if err = guard(); err != nil {
		return "", err
	}
	derived := parse.Parse("", []byte(md), engine.ParseOptions)
	if theme != nil {
		styles, e := theme()
		if e != nil {
			return "", e
		}
		fillThemeStyleValues(derived, styles)
	}
	if err = guard(); err != nil {
		return "", err
	}
	engine.RenderOptions.ProtyleMarkNetImg = false
	html = engine.ProtylePreview(derived, engine.RenderOptions)
	if note := derived.Root.ChildByType(ast.NodeFootnotesDefBlock); note != nil {
		note.Unlink()
	}
	if err = guard(); err != nil {
		return "", err
	}
	return html, nil
}
