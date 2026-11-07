// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
)

func attributeViewDefaultValues(attrView *av.AttributeView, view, groupView *av.View, previousItemID, addingItemID string, isCreate bool, env *attributeViewDefaultEnvironment) (ret map[string]*av.Value, err error) {
	return editcore.AttributeViewDefaultValues(attrView, view, groupView, previousItemID, addingItemID, isCreate, env.r04Core())
}

func fillAttributeViewDefaultValues(attrView *av.AttributeView, view, groupView *av.View, previousItemID, addingItemID string, isCreate bool, env *attributeViewDefaultEnvironment) error {
	return editcore.FillAttributeViewDefaultValues(attrView, view, groupView, previousItemID, addingItemID, isCreate, env.r04Core())
}

func validAttributeViewDefaultFilter(f *av.ViewFilter) bool {
	return editcore.ValidAttributeViewDefaultFilter(f)
}
