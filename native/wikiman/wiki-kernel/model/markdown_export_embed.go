// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func exportEmbedQuery(stmt string, ports []*exportReferenceRenderer) []indexdb.BlockRecord {
	if len(ports) == 0 {
		out := []indexdb.BlockRecord{}
		for _, b := range sql.SelectBlocksRawStmt(stmt, 1, Conf.Search.Limit) {
			out = append(out, indexdb.BlockRecord(*b))
		}
		return out
	}
	if !exportRenderReady(ports) {
		return nil
	}
	rows, err := ports[0].embedQuery(stmt)
	exportRenderFailure(ports, err)
	if !exportRenderReady(ports) {
		return nil
	}
	return rows
}
func exportEmbedLoad(id string, ports []*exportReferenceRenderer) (*parse.Tree, error) {
	if len(ports) == 0 {
		return LoadTreeByBlockID(id)
	}
	if err := ports[0].check(); err != nil {
		return nil, err
	}
	tree, err := ports[0].embedLoad(id)
	exportRenderFailure(ports, err)
	return tree, ports[0].check()
}
func exportEmbedHeadingMode(ports []*exportReferenceRenderer) int {
	if len(ports) == 0 {
		return Conf.Editor.HeadingEmbedMode
	}
	return ports[0].headingMode
}
func exportEmbedFormat(n *ast.Node, engine *lute.Lute, ports []*exportReferenceRenderer) string {
	text, err := lute.FormatNodeSync(n, engine.ParseOptions, engine.RenderOptions)
	if len(ports) > 0 {
		exportRenderFailure(ports, err)
	}
	return text
}
func (m *markdownExportContext) selectEmbeddedBlocks(stmt string) ([]indexdb.BlockRecord, error) {
	if err := m.guard(); err != nil {
		return nil, err
	}
	rows, err := indexdb.SelectReadBlockRows(m.read.binding.scope.ctx, m.read.binding.scope.commit.r.main.DB, stmt, 1, m.searchLimit)
	if err == nil {
		err = m.guard()
	}
	return rows, err
}

// Heading adjustment and format/parse must not mutate another embedding's
// candidate. Decode each use from the exact bytes admitted on first read.
func (m *markdownExportContext) embeddedTree(id string) (*parse.Tree, error) {
	p := m.referenceRenderer()
	bt, err := p.lookup(id)
	if err != nil {
		return nil, err
	}
	if bt == nil {
		return nil, ErrBlockNotFound
	}
	tree, err := m.exportTreeAt(bt.BoxID, bt.Path)
	if err != nil {
		return nil, err
	}
	if tree.ID != bt.RootID {
		return nil, errors.New("export_embed_root_mismatch")
	}
	return tree, nil
}

// An independent candidate from first-read source bytes, including documents
// discovered by directory traversal before they have an index entry.
func (m *markdownExportContext) exportTreeAt(box, p string) (*parse.Tree, error) {
	trees, err := m.read.selectedTrees(box, []string{p})
	if err != nil {
		return nil, err
	}
	id := util.GetTreeID(p)
	original := trees[id]
	saved, ok := m.read.documents[id]
	if !ok || original == nil {
		return nil, errors.New("export_embed_root_missing")
	}
	if err = saved.check(original, box, p); err != nil {
		return nil, err
	}
	tree, err := dataparser.ParseJSONWithoutFix(saved.raw, m.referenceRenderer().parser().ParseOptions)
	if err != nil {
		return nil, err
	}
	if tree == nil || tree.Root == nil || tree.ID != id {
		return nil, errors.New("export_embed_root_mismatch")
	}
	tree.Box, tree.Path, tree.HPath = box, p, original.HPath
	tree.Root.Box, tree.Root.Path = box, p
	if tree.Root.Spec == "" {
		parse.NestedInlines2FlattedSpans(tree, false)
		tree.Root.Spec = "1"
	}
	return tree, m.guard()
}
