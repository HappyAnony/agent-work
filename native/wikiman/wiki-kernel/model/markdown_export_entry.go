// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/treenode"
)

// Nil modes select the captured preferences. Handler-specific boolean defaults
// remain at the HTTP boundary; no renderer consults the current global settings.
type MarkdownContentOptions struct {
	RefMode, EmbedMode                                                      *int
	YFM, FillCSSVar, AdjustHeadingLevel, ImgTag, AssetsDestSpace2Underscore bool
}

func ExportMarkdownContentContext(ctx context.Context, id string, options MarkdownContentOptions) (string, string, error) {
	m, err := captureMarkdownExport(ctx)
	if err != nil {
		return "", "", err
	}
	return m.content(id, options, false)
}
func ExportStdMarkdownContext(ctx context.Context, id string, options MarkdownContentOptions) (string, error) {
	m, err := captureMarkdownExport(ctx)
	if err != nil {
		return "", err
	}
	_, content, err := m.content(id, options, true)
	return content, err
}
func (m *markdownExportContext) content(id string, options MarkdownContentOptions, standard bool) (string, string, error) {
	tree, _, err := m.prepareFocusedExport(id)
	if err != nil {
		return "", "", err
	}
	hpath := tree.HPath
	refMode, embedMode := m.settings.BlockRefMode, m.settings.BlockEmbedMode
	if options.RefMode != nil {
		refMode = *options.RefMode
	}
	if options.EmbedMode != nil {
		embedMode = *options.EmbedMode
	}
	p := m.referenceRenderer()
	var definitions []string
	cloudBase := ""
	if standard {
		cloudBase = m.cloudAssetsBase
		if refMode == 4 {
			seen := map[string]bool{}
			ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
				if !enter {
					return ast.WalkContinue
				}
				if p.check() != nil {
					return ast.WalkStop
				}
				var id string
				if treenode.IsBlockLink(n) {
					id = n.TextMarkAHref[len("siyuan://blocks/"):]
				} else if treenode.IsBlockRef(n) {
					id, _, _ = treenode.GetBlockRef(n)
				}
				if id != "" && !seen[id] {
					bt, err := p.lookup(id)
					if err != nil {
						p.err = err
						return ast.WalkStop
					}
					if bt != nil {
						seen[id] = true
						definitions = append(definitions, id)
					}
				}
				return ast.WalkContinue
			})
		}
	}
	if err = p.check(); err != nil {
		return "", "", err
	}
	content := exportMarkdownContent0(id, tree, cloudBase, options.AssetsDestSpace2Underscore, options.AdjustHeadingLevel, options.ImgTag, ".md", refMode, embedMode, m.settings.FileAnnotationRefMode, m.settings.TagOpenMarker, m.settings.TagCloseMarker, m.settings.BlockRefTextLeft, m.settings.BlockRefTextRight, m.settings.AddTitle, m.settings.InlineMemo, definitions, true, options.FillCSSVar, map[string]*parse.Tree{}, p)
	if err = p.check(); err != nil {
		return "", "", err
	}
	if !standard && options.YFM {
		content = yfm(parse.IAL2Map(tree.Root.KramdownIAL)) + content
	}
	if err = m.guard(); err != nil {
		return "", "", err
	}
	return hpath, content, nil
}
