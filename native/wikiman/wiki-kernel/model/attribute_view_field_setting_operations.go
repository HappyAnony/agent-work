// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"wiki-kernel/internal/editcore"
)

// Original field-setting algorithms; owner services stage all effects.

func setAttributeViewColWidthWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttributeViewColWidthWithEnvironment(operation, env.r04Core())
}

func setAttributeViewColWrapWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttributeViewColWrapWithEnvironment(operation, env.r04Core())
}

func setAttributeViewColHiddenWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttributeViewColHiddenWithEnvironment(operation, env.r04Core())
}

func setAttributeViewColPinWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttributeViewColPinWithEnvironment(operation, env.r04Core())
}

func setAttributeViewColIconWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttributeViewColIconWithEnvironment(operation, env.r04Core())
}

func setAttributeViewColDescWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttributeViewColDescWithEnvironment(operation, env.r04Core())
}

func updateAttributeViewColNumberFormatWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.UpdateAttributeViewColNumberFormatWithEnvironment(operation, env.r04Core())
}

func setAttributeViewColumnCalcWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttributeViewColumnCalcWithEnvironment(operation, env.r04Core())
}

func setAttributeViewColDateFillCreatedWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttributeViewColDateFillCreatedWithEnvironment(operation, env.r04Core())
}

func setAttrViewColDateFillSpecificTimeWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttrViewColDateFillSpecificTimeWithEnvironment(operation, env.r04Core())
}

func setAttrViewCreatedIncludeTimeWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttrViewCreatedIncludeTimeWithEnvironment(operation, env.r04Core())
}

func setAttrViewUpdatedIncludeTimeWithEnvironment(operation *Operation, env *attributeViewSettingsEnvironment) (err error) {
	return editcore.SetAttrViewUpdatedIncludeTimeWithEnvironment(operation, env.r04Core())
}
