// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
)

func sortAttributeViewGroupWithEnvironment(avID, blockID, previousGroupID, groupID string, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SortAttributeViewGroupWithEnvironment(avID, blockID, previousGroupID, groupID, env.r04Core())
}

func removeAttributeViewGroupWithEnvironment(avID, blockID string, env *attributeViewManagementEnvironment) (err error) {
	return editcore.RemoveAttributeViewGroupWithEnvironment(avID, blockID, env.r04Core())
}

func syncAttrViewTableColWidthWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SyncAttrViewTableColWidthWithEnvironment(operation, env.r04Core())
}

func hideAttributeViewGroupWithEnvironment(avID, blockID, groupID string, hidden int, env *attributeViewManagementEnvironment) (err error) {
	return editcore.HideAttributeViewGroupWithEnvironment(avID, blockID, groupID, hidden, env.r04Core())
}

func hideAttributeViewAllGroupsWithEnvironment(avID, blockID string, hidden bool, env *attributeViewManagementEnvironment) (err error) {
	return editcore.HideAttributeViewAllGroupsWithEnvironment(avID, blockID, hidden, env.r04Core())
}

func foldAttrViewGroupWithEnvironment(avID, blockID, groupID string, folded bool, env *attributeViewManagementEnvironment) (err error) {
	return editcore.FoldAttrViewGroupWithEnvironment(avID, blockID, groupID, folded, env.r04Core())
}

func configureAttributeViewGroupWithEnvironment(avID, blockID string, group *av.ViewGroup, env *attributeViewManagementEnvironment) (err error) {
	return editcore.ConfigureAttributeViewGroupWithEnvironment(avID, blockID, group, env.r04Core())
}

func setAttrViewCardAspectRatioWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttrViewCardAspectRatioWithEnvironment(operation, env.r04Core())
}

func changeAttrViewLayoutWithEnvironment(blockID, avID string, newLayout av.LayoutType, env *attributeViewManagementEnvironment) (err error) {
	return editcore.ChangeAttrViewLayoutWithEnvironment(blockID, avID, newLayout, env.r04Core())
}

func setAttrViewWrapFieldWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttrViewWrapFieldWithEnvironment(operation, env.r04Core())
}

func setAttrViewShowIconWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttrViewShowIconWithEnvironment(operation, env.r04Core())
}

func setAttrViewFitImageWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttrViewFitImageWithEnvironment(operation, env.r04Core())
}

func setAttrViewDisplayFieldNameWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttrViewDisplayFieldNameWithEnvironment(operation, env.r04Core())
}

func setAttrViewFillColBackgroundColorWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttrViewFillColBackgroundColorWithEnvironment(operation, env.r04Core())
}

func setAttrViewCardSizeWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttrViewCardSizeWithEnvironment(operation, env.r04Core())
}

func setAttrViewCoverFromAssetKeyIDWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttrViewCoverFromAssetKeyIDWithEnvironment(operation, env.r04Core())
}

func setAttrViewCoverFromWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttrViewCoverFromWithEnvironment(operation, env.r04Core())
}

func hideAttrViewNameWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.HideAttrViewNameWithEnvironment(operation, env.r04Core())
}

func addAttrViewViewWithEnvironment(avID, viewID, blockID string, layout av.LayoutType, env *attributeViewManagementEnvironment) (err error) {
	return editcore.AddAttrViewViewWithEnvironment(avID, viewID, blockID, layout, env.r04Core())
}

func setDatabaseBlockViewWithEnvironment(blockID, avID, viewID string, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetDatabaseBlockViewWithEnvironment(blockID, avID, viewID, env.r04Core())
}

func doSortAttrViewViewWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.DoSortAttrViewViewWithEnvironment(operation, env.r04Core())
}

func doRemoveAttrViewViewWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.DoRemoveAttrViewViewWithEnvironment(operation, env.r04Core())
}

func doDuplicateAttrViewViewWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.DoDuplicateAttrViewViewWithEnvironment(operation, env.r04Core())
}

func doSetAttrViewViewNameWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.DoSetAttrViewViewNameWithEnvironment(operation, env.r04Core())
}

func doSetAttrViewViewIconWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.DoSetAttrViewViewIconWithEnvironment(operation, env.r04Core())
}

func doSetAttrViewViewDescWithEnvironment(operation *Operation, env *attributeViewManagementEnvironment) (err error) {
	return editcore.DoSetAttrViewViewDescWithEnvironment(operation, env.r04Core())
}

func setAttributeViewGroupWithEnvironment(attrView *av.AttributeView, view *av.View, group *av.ViewGroup, env *attributeViewManagementEnvironment) (err error) {
	return editcore.SetAttributeViewGroupWithEnvironment(attrView, view, group, env.r04Core())
}
