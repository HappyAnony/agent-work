// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package avrender

import (
	"github.com/88250/lute/ast"
	"time"
	av "wiki-kernel/internal/avmodel"
)

type GroupEnvironment struct {
	MaxGroups int
	Now       func() time.Time
	Render    func(*av.AttributeView, *av.View, string) (av.Viewable, error)
	Read      func(string) (*av.AttributeView, error)
	NewView   func(av.LayoutType) (*av.View, error)
}

// Group view names are replaced by group labels. Other original constructor
// defaults (including fresh IDs, empty rules, page size and layout) are retained.
func NewGroupView(t av.LayoutType) (*av.View, error) {
	v := &av.View{ID: ast.NewNodeID(), Filters: []*av.ViewFilter{}, Sorts: []*av.ViewSort{}, PageSize: av.ViewDefaultPageSize, LayoutType: t}
	switch t {
	case av.LayoutTypeTable:
		v.Table = av.NewLayoutTable()
	case av.LayoutTypeGallery:
		v.Gallery = av.NewLayoutGallery()
	case av.LayoutTypeKanban:
		v.Kanban = av.NewLayoutKanban()
	default:
		return nil, ErrUnsupported
	}
	return v, nil
}
