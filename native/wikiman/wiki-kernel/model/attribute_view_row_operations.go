// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"

	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
)

func addAttributeViewBlockWithEnvironment(now int64, avID, dbBlockID, viewID, groupID, previousItemID, addingItemID, addingBoundBlockID, addingBlockContent string, isDetached, ignoreDefaultFill bool, tree *parse.Tree, env *attributeViewRowEnvironment) (err error) {
	return editcore.AddAttributeViewBlockWithEnvironment(now, avID, dbBlockID, viewID, groupID, previousItemID, addingItemID, addingBoundBlockID, addingBlockContent, isDetached, ignoreDefaultFill, tree, env.r04Core())
}

func replaceAttributeViewBlockWithEnvironment(attrView *av.AttributeView, oldBlockID, newNodeID string, isDetached bool, env *attributeViewRowEnvironment) (err error) {
	return editcore.ReplaceAttributeViewBlockWithEnvironment(attrView, oldBlockID, newNodeID, isDetached, env.r04Core())
}

func removeAttributeViewBlockWithEnvironment(srcIDs []string, avID string, env *attributeViewRowEnvironment) (err error) {
	return editcore.RemoveAttributeViewBlockWithEnvironment(srcIDs, avID, env.r04Core())
}

func removeNodeAvIDWithEnvironment(node *ast.Node, avID string, tree *parse.Tree, env *attributeViewRowEnvironment) (err error) {
	return editcore.RemoveNodeAvIDWithEnvironment(node, avID, tree, env.r04Core())
}
