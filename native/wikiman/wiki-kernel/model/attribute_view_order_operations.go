// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
)

func sortAttributeViewRowWithEnvironment(operation *Operation, env *attributeViewOrderEnvironment) (err error) {
	return editcore.SortAttributeViewRowWithEnvironment(operation, env.r04Core())
}

func sortAttributeViewViewKeyWithEnvironment(avID, blockID, keyID, previousKeyID string, env *attributeViewOrderEnvironment) (err error) {
	return editcore.SortAttributeViewViewKeyWithEnvironment(avID, blockID, keyID, previousKeyID, env.r04Core())
}

func sortAttributeViewKeyWithEnvironment(avID, keyID, previousKeyID string, env *attributeViewOrderEnvironment) (err error) {
	return editcore.SortAttributeViewKeyWithEnvironment(avID, keyID, previousKeyID, env.r04Core())
}

func duplicateAttributeViewKeyWithEnvironment(operation *Operation, env *attributeViewOrderEnvironment) (err error) {
	return editcore.DuplicateAttributeViewKeyWithEnvironment(operation, env.r04Core())
}

func refreshAttributeViewKeyIDs(attrView *av.AttributeView) {
	editcore.RefreshAttributeViewKeyIDs(attrView)
}
