// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"wiki-kernel/internal/editcore"
)

func updateAttributeViewColumnOptionsWithEnvironment(operation *Operation, e *attributeViewOptionEnvironment) (err error) {
	return editcore.UpdateAttributeViewColumnOptionsWithEnvironment(operation, e.r04Core())
}

func removeAttributeViewColumnOptionWithEnvironment(operation *Operation, e *attributeViewOptionEnvironment) (err error) {
	return editcore.RemoveAttributeViewColumnOptionWithEnvironment(operation, e.r04Core())
}

func updateAttributeViewColumnOptionWithEnvironment(operation *Operation, e *attributeViewOptionEnvironment) (err error) {
	return editcore.UpdateAttributeViewColumnOptionWithEnvironment(operation, e.r04Core())
}

func setAttributeViewColumnOptionDescWithEnvironment(operation *Operation, e *attributeViewOptionEnvironment) (err error) {
	return editcore.SetAttributeViewColumnOptionDescWithEnvironment(operation, e.r04Core())
}
