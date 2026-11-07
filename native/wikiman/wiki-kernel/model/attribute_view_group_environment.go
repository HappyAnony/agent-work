// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
)

func legacyGroupView(t av.LayoutType) (*av.View, error) {
	switch t {
	case av.LayoutTypeTable:
		return av.NewTableView(), nil
	case av.LayoutTypeGallery:
		return av.NewGalleryView(), nil
	case av.LayoutTypeKanban:
		return av.NewKanbanView(), nil
	}
	return nil, avrender.ErrUnsupported
}
