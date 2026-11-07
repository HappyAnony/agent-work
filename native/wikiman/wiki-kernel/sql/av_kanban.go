// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import (
	"wiki-kernel/av"
	"wiki-kernel/internal/avrender"
)

func RenderAttributeViewKanban(a *av.AttributeView, v *av.View, q string, depth *int, cached map[string]*av.AttributeView) *av.Kanban {
	ret, err := avrender.Kanban(a, v, q, legacyAVRenderEnvironment(depth, cached))
	if err != nil {
		return nil
	}
	return ret
}
