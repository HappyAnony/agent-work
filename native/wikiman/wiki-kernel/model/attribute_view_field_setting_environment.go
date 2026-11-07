// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"wiki-kernel/internal/editcore"
	// This migration seam does not save source or emit global notifications.
)

var attributeViewFieldSettingOperations = map[string]func(*Operation, *attributeViewSettingsEnvironment) error{
	"setAttrViewColWidth":                setAttributeViewColWidthWithEnvironment,
	"setAttrViewColWrap":                 setAttributeViewColWrapWithEnvironment,
	"setAttrViewColHidden":               setAttributeViewColHiddenWithEnvironment,
	"setAttrViewColPin":                  setAttributeViewColPinWithEnvironment,
	"setAttrViewColIcon":                 setAttributeViewColIconWithEnvironment,
	"setAttrViewColDesc":                 setAttributeViewColDescWithEnvironment,
	"updateAttrViewColNumberFormat":      updateAttributeViewColNumberFormatWithEnvironment,
	"setAttrViewColCalc":                 setAttributeViewColumnCalcWithEnvironment,
	"setAttrViewColDateFillCreated":      setAttributeViewColDateFillCreatedWithEnvironment,
	"setAttrViewColDateFillSpecificTime": setAttrViewColDateFillSpecificTimeWithEnvironment,
	"setAttrViewCreatedIncludeTime":      setAttrViewCreatedIncludeTimeWithEnvironment,
	"setAttrViewUpdatedIncludeTime":      setAttrViewUpdatedIncludeTimeWithEnvironment,
}

func isAttributeViewFieldSettingAction(action string) bool {
	return editcore.IsAttributeViewFieldSettingAction(action)
}

func attributeViewFieldSettingNeedsView(action string) bool {
	return editcore.AttributeViewFieldSettingNeedsView(action)
}

func validateAttributeViewFieldSettingInput(op *Operation) error {
	return editcore.ValidateAttributeViewFieldSettingInput(op)
}

func runAttributeViewFieldSetting(ctx context.Context, op *Operation, e *attributeViewSettingsEnvironment) error {
	return editcore.RunAttributeViewFieldSetting(ctx, op, e.r04Core())
}

func runLegacyAttributeViewFieldSetting(action string, op *Operation) error {
	if op == nil {
		return errors.New("invalid_av_field_setting_operation")
	}
	copy := *op
	copy.Action = action
	return runAttributeViewFieldSetting(context.Background(), &copy, legacyAttributeViewSettingsEnvironment())
}

func (tx *Transaction) doAttributeViewFieldSetting(action string, op *Operation) *TxErr {
	if op == nil {
		return &TxErr{code: TxErrHandleAttributeView, msg: "invalid_av_field_setting_operation"}
	}
	copy := *op
	copy.Action = action
	ctx := context.Background()
	var e *attributeViewSettingsEnvironment
	if tx.environment == nil || tx.environment.legacy {
		e = legacyAttributeViewSettingsEnvironment()
	} else if tx.environment.avConfig != nil {
		e = tx.attributeViewSettingsEnvironment()
	}
	if tx.environment != nil {
		ctx = tx.environment.ctx
	}
	if err := runAttributeViewFieldSetting(ctx, &copy, e); err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: op.AvID, msg: err.Error()}
	}
	return nil
}
