// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"path"
	"wiki-kernel/util"
)

type htmlExportFileOptions struct{ Markdown, PDF, Docx, KeepFold, Merge bool }
type htmlExportFile struct {
	name, content string
	output        *archiveExportOutput
}

// The controller supplies its final consumer; no caller-chosen destination is
// admitted here. Borrowed output is live only during consume. Failure removes
// this attempt's output; a successful ZIP consumer may keep its checked
// publication. Frontend document wrapping remains outside this port.
func (m *markdownExportContext) withHTMLFile(id string, options htmlExportFileOptions, consume func(htmlExportFile) error) (err error) {
	if err = m.guard(); err != nil {
		return err
	}
	if consume == nil || (options.Markdown && options.PDF) || (!options.Markdown && options.Docx) || (options.Markdown && options.KeepFold) {
		return errors.New("html_file_options_invalid")
	}
	var tree *parse.Tree
	var name, content string
	if options.Markdown {
		tree, name, content, err = m.renderMarkdownHTMLDocument(id, options.Docx, options.Merge)
	} else {
		tree, _, name, content, err = m.renderHTMLDocument(id, options.PDF, options.KeepFold, options.Merge)
	}
	if err != nil {
		return err
	}
	o, err := m.newOutput(name, ".html")
	if err != nil {
		return err
	}
	keep := false
	defer func() { err = errors.Join(err, o.finish(keep)) }()
	if err = m.copyHTMLResources(o, tree, !options.PDF); err != nil {
		return err
	}
	if err = m.guard(); err != nil {
		return err
	}
	if err = consume(htmlExportFile{name: name, content: content, output: o}); err != nil {
		return err
	}
	if err = m.guard(); err != nil {
		return err
	}
	keep = o.published
	return nil
}

func (m *markdownExportContext) renderMarkdownHTMLDocument(id string, docx, merge bool) (tree *parse.Tree, name, content string, err error) {
	tree, _, _, err = m.prepareHTMLDocument(id, merge)
	if err != nil {
		return nil, "", "", err
	}
	fail := func(e error) (*parse.Tree, string, string, error) { return nil, "", "", e }
	p := m.referenceRenderer()
	tree = exportTree(tree, true, false, true, m.settings.BlockRefMode, m.settings.BlockEmbedMode, m.settings.FileAnnotationRefMode, m.settings.TagOpenMarker, m.settings.TagCloseMarker, m.settings.BlockRefTextLeft, m.settings.BlockRefTextRight, m.settings.AddTitle, m.settings.InlineMemo, true, true, map[string]*parse.Tree{}, p)
	if err = p.check(); err != nil {
		return fail(err)
	}
	name = util.FilterFileName(path.Base(tree.HPath))
	if docx {
		if err = processExportIFrames(tree, m.guard); err != nil {
			return fail(err)
		}
	}
	if err = m.checkExportRenderTree(tree); err != nil {
		return fail(err)
	}
	content, err = renderMarkdownExportHTML(tree, p.parser(), docx, m.guard)
	if err != nil {
		return fail(err)
	}
	return tree, name, content, nil
}

// Retain the original Markdown-HTML/Word normalization and renderer selection.
// This branch does not run ExportHTML's heading-level adjustment.
func renderMarkdownExportHTML(tree *parse.Tree, engine *lute.Lute, docx bool, guard func() error) (string, error) {
	if tree == nil || tree.Root == nil || engine == nil || guard == nil {
		return "", errors.New("markdown_html_render_dependencies_unbound")
	}
	if err := guard(); err != nil {
		return "", err
	}
	engine.SetFootnotes(true)
	var err error
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if err = guard(); err != nil {
			return ast.WalkStop
		}
		if !enter {
			return ast.WalkContinue
		}
		if n.Type == ast.NodeEmojiImg {
			n.Tokens = bytes.ReplaceAll(n.Tokens, []byte("src=\"/emojis"), []byte("src=\"emojis"))
		} else if n.Type == ast.NodeList {
			if n.ListData != nil && n.ListData.Typ == 1 {
				if n.ListData.Start == 0 {
					n.ListData.Start = 1
				}
				if li := n.ChildByType(ast.NodeListItem); li != nil && li.ListData != nil {
					n.ListData.Start = li.ListData.Num
				}
			}
		} else if n.IsTextMarkType("code") && n.Next != nil && n.Next.Type == ast.NodeText {
			n.Next.Tokens = bytes.TrimPrefix(n.Next.Tokens, []byte(editor.Zwsp))
		}
		return ast.WalkContinue
	})
	if err != nil {
		return "", err
	}
	var content string
	if docx {
		content = string(render.NewProtyleExportDocxRenderer(tree, engine.RenderOptions).Render())
	} else {
		content = engine.ProtylePreview(tree, engine.RenderOptions)
	}
	if err = guard(); err != nil {
		return "", err
	}
	return content, nil
}
