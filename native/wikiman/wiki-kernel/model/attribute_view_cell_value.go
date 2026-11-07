// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
)

func attrViewKeyType(a *av.AttributeView, id string) av.KeyType {
	return editcore.AttrViewKeyType(a, id)
}

func updateAttributeViewValueWithEnvironment(e *attributeViewCellEnvironment, attrView *av.AttributeView, keyID, itemID string, valueData interface{}) (val *av.Value, err error) {
	return editcore.UpdateAttributeViewValueWithEnvironment(e.r04Core(), attrView, keyID, itemID, valueData)
}

func updateTwoWayRelationDestWithEnvironment(e *attributeViewCellEnvironment, attrView *av.AttributeView, relKey *av.Key, val *av.Value, relationChangeMode int, oldRelationBlockIDs []string) (err error) {
	return editcore.UpdateTwoWayRelationDestWithEnvironment(e.r04Core(), attrView, relKey, val, relationChangeMode, oldRelationBlockIDs)
}
