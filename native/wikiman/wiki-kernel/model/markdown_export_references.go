// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
)

// This per-render adapter bridges the original value-only renderer helpers.
// Its first failure is returned by the owned Markdown entry, never downgraded
// to a missing reference or a fallback to a current process-global resource.
type exportReferenceRenderer struct {
	attributeTable func(string, string) (*av.AttributeView, *av.Table, error)
	guard          func() error
	title          func(*parse.Tree) (*Block, error)
	plainParser    func() *lute.Lute
	theme          func(*parse.Tree) error
	annotation     func(string) (string, int, error)
	embedQuery     func(string) ([]indexdb.BlockRecord, error)
	embedLoad      func(string) (*parse.Tree, error)
	headingMode    int
	lookup         func(string) (*treenode.BlockTree, error)
	load           func(string, map[string]*parse.Tree) (*parse.Tree, error)
	text           func(string) (string, error)
	parser         func() *lute.Lute
	max            int
	err            error
}

func (p *exportReferenceRenderer) check() error {
	if p.err == nil {
		p.err = p.guard()
	}
	return p.err
}
func exportRenderReady(ports []*exportReferenceRenderer) bool {
	return len(ports) == 0 || ports[0].check() == nil
}
func exportRenderFailure(ports []*exportReferenceRenderer, err error) {
	if len(ports) > 0 && ports[0].err == nil {
		ports[0].err = err
	}
}
func exportRenderParser(ports []*exportReferenceRenderer) *lute.Lute {
	if len(ports) == 0 {
		return NewLute()
	}
	return ports[0].parser()
}
func exportRenderAnchorMax(ports []*exportReferenceRenderer) int {
	if len(ports) == 0 {
		return Conf.Editor.BlockRefDynamicAnchorTextMaxLen
	}
	return ports[0].max
}
func exportRenderLookup(id string, ports []*exportReferenceRenderer) *treenode.BlockTree {
	if len(ports) == 0 {
		return treenode.GetBlockTree(id)
	}
	if !exportRenderReady(ports) {
		return nil
	}
	b, err := ports[0].lookup(id)
	exportRenderFailure(ports, err)
	if !exportRenderReady(ports) {
		return nil
	}
	return b
}
func exportRenderText(id string, ports []*exportReferenceRenderer) string {
	if len(ports) == 0 {
		return sql.GetRefText(id)
	}
	if !exportRenderReady(ports) {
		return ""
	}
	text, err := ports[0].text(id)
	exportRenderFailure(ports, err)
	if !exportRenderReady(ports) {
		return ""
	}
	return text
}
func exportRenderLoad(id string, cache map[string]*parse.Tree, ports []*exportReferenceRenderer) (*parse.Tree, error) {
	if len(ports) == 0 {
		return loadTreeWithCache(id, cache)
	}
	if err := ports[0].check(); err != nil {
		return nil, err
	}
	tree, err := ports[0].load(id, cache)
	exportRenderFailure(ports, err)
	return tree, ports[0].check()
}
func (m *markdownExportContext) referenceRenderer() *exportReferenceRenderer {
	r, ctx := m.read.binding.scope.commit.r, m.read.binding.scope.ctx
	parser := func() *lute.Lute { return nativeparser.NewEditor(m.parserOptions) }
	p := &exportReferenceRenderer{guard: m.guard, max: m.anchorMax, parser: parser, headingMode: m.headingMode}
	p.attributeTable = m.exportAttributeTable
	p.title = m.exportDocumentTitle
	p.plainParser = m.markdownParser
	p.theme = m.fillThemeStyle
	p.annotation = m.readAnnotationPage
	p.embedQuery = m.selectEmbeddedBlocks
	p.embedLoad = m.embeddedTree
	p.lookup = func(id string) (*treenode.BlockTree, error) {
		if err := m.guard(); err != nil {
			return nil, err
		}
		b, err := r.referenceLookup(ctx, id)
		if err != nil || b == nil {
			return nil, err
		}
		if _, err = m.read.box(b.BoxID); err != nil {
			return nil, err
		}
		return &treenode.BlockTree{ID: b.ID, RootID: b.RootID, ParentID: b.ParentID, BoxID: b.BoxID, Path: b.Path, HPath: b.HPath, Updated: b.Updated, Type: b.Type}, m.guard()
	}
	p.load = func(id string, cache map[string]*parse.Tree) (*parse.Tree, error) {
		b, err := p.lookup(id)
		if err != nil {
			return nil, err
		}
		if b == nil {
			return nil, ErrBlockNotFound
		}
		if t := cache[b.RootID]; t != nil {
			if t.ID != b.RootID || t.Box != b.BoxID || t.Path != b.Path {
				return nil, errors.New("export_reference_identity_changed")
			}
			// A focused candidate is the current render range, not the full source.
			// Definitions outside it must read an independent full source candidate.
			if _, focused := m.focusTrees[t]; focused && treenode.GetNodeInTree(t, b.ID) == nil {
				return m.embeddedTree(id)
			}
			return t, m.guard()
		}
		loaded, err := m.read.selectedTrees(b.BoxID, []string{b.Path})
		if err != nil {
			return nil, err
		}
		tree := loaded[b.RootID]
		if tree == nil {
			return nil, errors.New("export_reference_root_missing")
		}
		cache[b.RootID] = tree
		return tree, m.guard()
	}
	p.text = func(id string) (string, error) {
		text, err := indexdb.ReferenceText(ctx, id, nativeparser.NewIndex(m.parserOptions.Markdown), func(ctx context.Context, id string) (indexdb.BlockRecord, bool, error) {
			if err := m.guard(); err != nil {
				return indexdb.BlockRecord{}, false, err
			}
			row, ok, err := (indexdb.CachedBlocks{DB: r.main.DB}).Get(ctx, id)
			if err == nil {
				err = m.guard()
			}
			return row, ok, err
		})
		if err == nil {
			err = m.guard()
		}
		return text, err
	}
	return p
}
