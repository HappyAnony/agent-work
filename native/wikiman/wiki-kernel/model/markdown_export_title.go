// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/util"
)

func (m *markdownExportContext) markdownParser() *lute.Lute {
	return nativeparser.NewLute(m.parserOptions.Markdown)
}
func exportRenderMarkdownParser(ports []*exportReferenceRenderer) *lute.Lute {
	if len(ports) == 0 {
		return util.NewLute()
	}
	return ports[0].plainParser()
}
func exportRenderDocumentTitle(id string, tree *parse.Tree, ports []*exportReferenceRenderer) *Block {
	if len(ports) == 0 {
		root, _ := getBlock(id, tree)
		return root
	}
	p := ports[0]
	if p.check() != nil {
		return nil
	}
	if tree == nil || id != tree.ID {
		exportRenderFailure(ports, errors.New("export_title_identity_changed"))
		return nil
	}
	root, err := p.title(tree)
	exportRenderFailure(ports, err)
	if !exportRenderReady(ports) {
		return nil
	}
	return root
}
func (m *markdownExportContext) exportDocumentTitle(tree *parse.Tree) (*Block, error) {
	if err := m.guard(); err != nil {
		return nil, err
	}
	if tree == nil {
		return nil, errors.New("export_title_unbound")
	}
	if err := m.checkExportRenderTree(tree); err != nil {
		return nil, err
	}

	row, err := indexdb.ProjectDocumentBlock(m.read.binding.scope.ctx, tree, nativeparser.NewIndex(m.parserOptions.Markdown))
	if err != nil {
		return nil, err
	}
	block := blockview.FromRow(row, "", 0, m.searchCaseSensitive)
	if block == nil {
		return nil, errors.New("export_title_projection_missing")
	}
	if block.IAL == nil {
		block.IAL = map[string]string{}
	}
	return block, m.guard()
}
