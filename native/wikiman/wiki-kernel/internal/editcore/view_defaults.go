// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package editcore

import av "wiki-kernel/internal/avmodel"

func NewAttributeViewManagementView(t av.LayoutType, label func(string) string, id func() string) *av.View {
	base := func() *av.BaseLayout { return &av.BaseLayout{ID: id(), ShowIcon: true} }
	v := &av.View{ID: id(), Name: label(string(t)), Filters: []*av.ViewFilter{}, Sorts: []*av.ViewSort{}, PageSize: av.ViewDefaultPageSize, LayoutType: t}
	switch t {
	case av.LayoutTypeTable:
		v.Table = &av.LayoutTable{BaseLayout: base()}
	case av.LayoutTypeGallery:
		v.Gallery = &av.LayoutGallery{BaseLayout: base(), CoverFrom: av.CoverFromContentImage, CardAspectRatio: av.CardAspectRatio16_9, CardSize: av.CardSizeMedium}
	case av.LayoutTypeKanban:
		v.Kanban = &av.LayoutKanban{BaseLayout: base(), CoverFrom: av.CoverFromContentImage, CardAspectRatio: av.CardAspectRatio16_9, CardSize: av.CardSizeMedium}
	default:
		return nil
	}
	return v
}
