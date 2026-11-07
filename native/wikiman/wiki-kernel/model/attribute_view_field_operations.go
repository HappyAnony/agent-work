// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"wiki-kernel/internal/editcore"
)

// Original field algorithms; effects are supplied by the transaction owner.
func addAttributeViewKeyWithEnvironment(avID, keyID, keyName, keyType, keyIcon, previousKeyID string, env *attributeViewFieldEnvironment) (err error) {
	return editcore.AddAttributeViewKeyWithEnvironment(avID, keyID, keyName, keyType, keyIcon, previousKeyID, env.r04Core())
}

func updateAttributeViewColumnWithEnvironment(operation *Operation, env *attributeViewFieldEnvironment) (err error) {
	return editcore.UpdateAttributeViewColumnWithEnvironment(operation, env.r04Core())
}

func removeAttributeViewKeyWithEnvironment(avID, keyID string, removeRelationDest bool, env *attributeViewFieldEnvironment) (err error) {
	return editcore.RemoveAttributeViewKeyWithEnvironment(avID, keyID, removeRelationDest, env.r04Core())
}
