// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/filelock"
	"github.com/siyuan-note/logging"
	"os"
	"strings"
	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/sql"
)

func exportRenderAttributeTable(id, viewID string, ports []*exportReferenceRenderer) (*av.AttributeView, *av.Table) {
	if len(ports) > 0 {
		if !exportRenderReady(ports) {
			return nil, nil
		}
		p := ports[0]
		if p.attributeTable == nil {
			exportRenderFailure(ports, errors.New("export_av_port_unbound"))
			return nil, nil
		}
		a, t, err := p.attributeTable(id, viewID)
		exportRenderFailure(ports, err)
		if !exportRenderReady(ports) {
			return nil, nil
		}
		return a, t
	}
	// The original compatibility path skips missing/invalid AVs and logs errors.
	if !filelock.IsExist(av.GetAttributeViewDataPath(id)) {
		return nil, nil
	}
	a, err := av.ParseAttributeView(id)
	if err != nil {
		logging.LogErrorf("parse attribute view [%s] failed: %s", id, err)
		return nil, nil
	}
	v, err := a.GetCurrentView(viewID)
	if err != nil {
		logging.LogErrorf("get attribute view [%s] failed: %s", id, err)
		return nil, nil
	}
	t := getAttrViewTable(a, v, "")
	cache := map[string]*av.AttributeView{}
	further := sql.GetFurtherCollections(a, cache)
	av.Filter(t, a, further, cache)
	av.Sort(t, a)
	return a, t
}

func (m *markdownExportContext) attributeScope() (*commitAVContext, error) {
	if err := m.guard(); err != nil {
		return nil, err
	}
	if m.avScope != nil {
		return m.avScope, nil
	}
	s := m.read.binding.scope.commit.avContext(m.read.binding.scope.ctx)
	s.now = m.read.binding.scope.now
	s.readOnly = true
	s.sourceReader = func(name string) ([]byte, error) {
		if err := m.guard(); err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "storage/av/"), ".json")
		if name != "storage/av/"+id+".json" {
			return nil, avrender.ErrShape
		}
		a, err := m.read.readExportView(id)
		if err != nil {
			return nil, err
		}
		if a == nil {
			return nil, os.ErrNotExist
		}
		return append([]byte(nil), m.read.views[id].raw...), m.guard()
	}
	s.treeReader = func(ids []string) (map[string]*parse.Tree, error) {
		trees := map[string]*parse.Tree{}
		for _, id := range ids {
			t, err := m.embeddedTree(id)
			if errors.Is(err, ErrBlockNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			trees[id] = t
		}
		return trees, m.guard()
	}
	m.avScope = s
	return s, nil
}
func (m *markdownExportContext) exportAttributeTable(id, viewID string) (*av.AttributeView, *av.Table, error) {
	if err := m.guard(); err != nil {
		return nil, nil, err
	}
	original, err := m.read.readExportView(id)
	if err != nil || original == nil {
		return nil, nil, err
	} // Missing AV remains a dangling block.
	s, err := m.attributeScope()
	if err != nil {
		return nil, nil, err
	}
	// Start each table conversion from the shared original bytes. Rendering may
	// repair candidates and fill auto-values; those mutations never become source.
	s.cached = map[string]*av.AttributeView{}
	a, err := s.read(id)
	if err != nil {
		return nil, nil, err
	}
	if err = validateAttributeViewFieldSource(a); err != nil {
		return nil, nil, err
	}
	v, err := a.GetCurrentView(viewID)
	if err != nil {
		return nil, nil, err
	}
	if err = validateAttributeViewPreviewLayout(v); err != nil {
		return nil, nil, err
	}
	projectAttributeViewTable(v)
	out, err := s.renderAs(a, v, "", av.LayoutTypeTable)
	if err != nil {
		return nil, nil, err
	}
	t, ok := out.(*av.Table)
	if !ok {
		return nil, nil, avrender.ErrShape
	}
	// Projection is a table even when the saved view is gallery/kanban. Do not
	// expose that temporary cache entry to layout-specific related rendering.
	if v.LayoutType != av.LayoutTypeTable {
		delete(a.RenderedViewables, v.ID)
	}
	cache := map[string]*av.AttributeView{}
	further, err := avrender.FurtherCollections(s.ctx, a, cache, avrender.FurtherEnvironment{Read: s.read, Render: s.render, Missing: os.IsNotExist})
	if err != nil {
		return nil, nil, err
	}
	if err = avmodel.FilterChecked(t, a, further, cache); err != nil {
		return nil, nil, err
	}
	avmodel.Sort(t, a)
	if err = m.guard(); err != nil {
		return nil, nil, err
	}
	return a, t, nil
}
