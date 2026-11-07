// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/treenode"
	// This seam stages detached candidates only. Source commit belongs to R04.
)

type attributeViewSettingsEnvironment struct {
	complete func() error
	read     func(string) (*av.AttributeView, error)
	view     func(*av.AttributeView, string) (*av.View, error)
	stage    func(*av.AttributeView) error
}

func isAttributeViewSettingsAction(action string) bool {
	return action == "setAttrViewFilters" || action == "setAttrViewSorts" || action == "setAttrViewPageSize"
}

func validateAttributeViewSettings(op *Operation) error {
	return editcore.ValidateAttributeViewSettings(op)
}

func legacyAttributeViewSettingsEnvironment(contexts ...context.Context) *attributeViewSettingsEnvironment {
	ctx := legacyAVContext(contexts)
	return settingsWithBinding(newLegacyAVBinding(ctx))
}
func settingsWithBinding(b *legacyAVBinding) *attributeViewSettingsEnvironment {
	return &attributeViewSettingsEnvironment{read: b.readCandidate, stage: b.saveUpgraded, view: b.readView, complete: b.complete}
}

func (tx *Transaction) attributeViewSettingsEnvironment() *attributeViewSettingsEnvironment {
	if tx == nil || tx.environment == nil || tx.environment.legacy {
		b := legacyAVBindingForTransaction(tx)
		e := settingsWithBinding(b)
		if tx != nil && tx.environment != nil && tx.environment.commitContext != nil {
			e.view = func(a *av.AttributeView, id string) (*av.View, error) {
				if err := b.guard(); err != nil {
					return nil, err
				}
				return selectAttributeViewReadView(a, id, &attributeViewReadEnvironment{ctx: tx.environment.ctx, legacy: true, strict: true, ownerGuard: b.guard, node: func(id string) (*ast.Node, *parse.Tree, error) { return b.node(tx, id) }})
			}
		}
		return e
	}
	e := tx.environment
	return &attributeViewSettingsEnvironment{read: e.avConfig.read, stage: e.avConfig.stage, view: func(a *av.AttributeView, blockID string) (*av.View, error) {
		if err := e.ctx.Err(); err != nil {
			return nil, err
		}
		var viewID string
		if blockID != "" {
			// Missing blocks retain the original current-view fallback. A known
			// source that fails to load must not silently select another view.
			bt, err := e.lookupBlock(blockID)
			if err != nil {
				return nil, err
			}
			if bt != nil {
				tree, err := tx.loadTreeByBlockTree(bt)
				if err != nil {
					return nil, err
				}
				if tree == nil || tree.Root == nil {
					return nil, errors.New("invalid_av_view_source")
				}
				if node := treenode.GetNodeInTree(tree, blockID); node != nil {
					viewID = node.IALAttr(av.NodeAttrView)
				}
			}
		}
		if err := e.ctx.Err(); err != nil {
			return nil, err
		}
		return a.GetCurrentView(viewID)
	}}
}

// Both legacy wrappers and original transaction receivers use the same algorithm.
func setAttributeViewSettings(ctx context.Context, op *Operation, e *attributeViewSettingsEnvironment) error {
	return editcore.SetAttributeViewSettings(ctx, op, e.r04Core())
}

func (tx *Transaction) doAttributeViewSettings(op *Operation) *TxErr {
	ctx := context.Background()
	if tx.environment != nil {
		ctx = tx.environment.ctx
	}
	if err := setAttributeViewSettings(ctx, op, tx.attributeViewSettingsEnvironment()); err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: op.AvID, msg: err.Error()}
	}
	return nil
}
