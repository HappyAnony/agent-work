// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"

	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/editcore"
)

// All scoped callbacks stage owned candidates/effects; R04 owns publication.

type attributeViewManagementEnvironment struct {
	ownerGuard         func() error
	strictDependencies bool
	complete           func() error
	ctx                context.Context
	legacy             bool
	read               func(string) (*av.AttributeView, error)
	stage              func(*av.AttributeView) error
	view               func(*av.AttributeView, string) (*av.View, error)
	groups             func(*av.AttributeView) error
	count              func(*av.AttributeView, *av.View, *av.View) (int, error)
	sortGroups         func(*av.AttributeView, *av.View) error
	makeView           func(av.LayoutType) *av.View
	language           func(string) string
	cloneView          func(*av.View) (*av.View, error)
	mirrors            func(string) ([]string, error)
	node               func(string) (*ast.Node, *parse.Tree, error)
	attrs              func(*ast.Node, *parse.Tree, map[string]string) error
	mirrorAttrs        func(*ast.Node, *parse.Tree, map[string]string) error
	flushTrees         func([]*parse.Tree) error
	reload             func(string) error
}

var attributeViewManagementActions = []string{
	"sortAttrViewGroup", "removeAttrViewGroup", "syncAttrViewTableColWidth", "hideAttrViewGroup", "hideAttrViewAllGroups", "foldAttrViewGroup", "setAttrViewGroup",
	"setAttrViewCardAspectRatio", "setAttrViewBlockView", "changeAttrViewLayout", "setAttrViewWrapField", "setAttrViewShowIcon", "setAttrViewFitImage", "setAttrViewDisplayFieldName", "setAttrViewFillColBackgroundColor", "setAttrViewCardSize", "setAttrViewCoverFromAssetKeyID", "setAttrViewCoverFrom", "hideAttrViewName", "addAttrViewView",
	"sortAttrViewView", "removeAttrViewView", "duplicateAttrViewView", "setAttrViewViewName", "setAttrViewViewIcon", "setAttrViewViewDesc",
}

func isAttributeViewManagementAction(action string) bool {
	return editcore.IsAttributeViewManagementAction(action)
}

func validateAttributeViewManagementInput(op *Operation) error {
	return editcore.ValidateAttributeViewManagementInput(op)
}

func validAttributeViewManagementLayout(t av.LayoutType) bool {
	return editcore.ValidAttributeViewManagementLayout(t)
}

func (e *attributeViewManagementEnvironment) validate(action string) error {
	return e.r04Core().Validate(action)
}

func attributeViewManagementError(err error) error { return editcore.AttributeViewManagementError(err) }

func cloneAttributeViewManagementValue(v *av.Value) *av.Value {
	return editcore.CloneAttributeViewManagementValue(v)
}

func cloneAttributeViewManagementView(v *av.View) (*av.View, error) {
	return editcore.CloneAttributeViewManagementView(v)
}

func legacyAttributeViewManagementEnvironment(contexts ...context.Context) *attributeViewManagementEnvironment {
	ctx := legacyAVContext(contexts)
	return managementWithBinding(nil, newLegacyAVBinding(ctx), ctx)
}
func managementWithBinding(tx *Transaction, b *legacyAVBinding, ctx context.Context) *attributeViewManagementEnvironment {
	label := func(key string) string { return b.scope.commit.avLabels[key] }
	// Failed bindings expose error ports, and never dereference a missing scope.
	if b.err != nil {
		label = func(string) string { return "" }
	}
	return &attributeViewManagementEnvironment{ctx: ctx, legacy: true, strictDependencies: true, ownerGuard: b.guard, complete: b.complete,
		read: b.readCandidate, stage: b.saveUpgraded, view: b.readView, groups: b.groups, count: b.count,
		sortGroups: func(a *av.AttributeView, v *av.View) error {
			return b.effect(func() error { avrender.SortGroups(a, v, b.scope.now); return nil })
		},
		makeView: func(t av.LayoutType) *av.View {
			if b.guard() != nil {
				return nil
			}
			return newAttributeViewManagementView(t, label, ast.NewNodeID)
		}, language: label, cloneView: cloneAttributeViewManagementView,
		mirrors: func(id string) ([]string, error) { return b.mirrors(tx, id) }, node: func(id string) (*ast.Node, *parse.Tree, error) { return b.node(tx, id) },
		attrs: func(n *ast.Node, t *parse.Tree, attrs map[string]string) error { return b.attributes(tx, n, t, attrs) },
		mirrorAttrs: func(n *ast.Node, _ *parse.Tree, attrs map[string]string) error {
			return b.effect(func() error {
				old, err := setNodeAttrs0(n, attrs)
				if err != nil {
					return err
				}
				cache.PutBlockIAL(n.ID, parse.IAL2Map(n.KramdownIAL))
				pushBroadcastAttrTransactions(old, n)
				return nil
			})
		},
		flushTrees: func(trees []*parse.Tree) error {
			return b.effect(func() error {
				for _, tree := range trees {
					var err error
					if tx != nil {
						err = tx.writeTree(tree)
					} else {
						err = indexWriteTreeUpsertQueue(tree)
					}
					if err != nil {
						return err
					}
				}
				return nil
			})
		}, reload: b.reload}
}

func (tx *Transaction) attributeViewManagementEnvironment() *attributeViewManagementEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avManagement == nil {
			return nil
		}
		e := *tx.environment.avManagement
		e.ctx = tx.environment.ctx
		e.legacy = false
		return &e
	}
	return managementWithBinding(tx, legacyAVBindingForTransaction(tx), transactionAVContext(tx))
}
func (tx *Transaction) doAttributeViewManagement(action string, op *Operation) *TxErr {
	local := *op
	local.Action = action
	if err := runAttributeViewManagement(&local, tx.attributeViewManagementEnvironment()); err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: op.AvID, msg: err.Error()}
	}
	op.RetData = local.RetData
	return nil
}
func runLegacyAttributeViewManagement(action string, op *Operation) error {
	local := *op
	local.Action = action
	return runAttributeViewManagement(&local, legacyAttributeViewManagementEnvironment())
}

func runAttributeViewManagement(op *Operation, e *attributeViewManagementEnvironment) error {
	return editcore.RunAttributeViewManagement(op, e.r04Core())
}

func updateAttributeViewManagementMirrors(e *attributeViewManagementEnvironment, a *av.AttributeView, old string, v *av.View) error {
	return editcore.UpdateAttributeViewManagementMirrors(e.r04Core(), a, old, v)
}

func newAttributeViewManagementView(t av.LayoutType, label func(string) string, id func() string) *av.View {
	return editcore.NewAttributeViewManagementView(t, label, id)
}
