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
	// Native effects are transaction candidates, not R04 source commits. Refresh is
	// synchronous in an owned transaction: errors/cancellation must reach its caller.
)

type attributeViewUpdateEnvironment struct {
	strictDependencies bool
	asyncRefresh       func([]string) error
	complete           func() error
	ctx                context.Context
	legacy             bool
	read               func(string) (*av.AttributeView, error)
	upsert             func(string, string) (bool, error)
	sources            func(string) ([]string, error)
	groups             func(*av.AttributeView) error
	stage              func(*av.AttributeView) error
	reload             func(string) error
	attrs              func(*ast.Node, *parse.Tree, map[string]string) error
	tree               func(*parse.Tree) error
}

func (e *attributeViewUpdateEnvironment) validate(update bool) error {
	return e.r04Core().Validate(update)
}

func legacyAttributeViewUpdateEnvironment(tx *Transaction) *attributeViewUpdateEnvironment {
	b := legacyAVBindingForTransaction(tx)
	return &attributeViewUpdateEnvironment{ctx: transactionAVContext(tx), legacy: true, strictDependencies: true, asyncRefresh: b.scheduleUpdated, complete: b.complete,
		read:    b.read,
		upsert:  b.registerMirror,
		sources: b.sources,
		groups:  b.groups,
		stage:   b.save,
		reload:  b.reload,
		attrs:   setNodeAttrs,
		tree: func(t *parse.Tree) error {
			if tx == nil {
				return errors.New("av_update_transaction_unbound")
			}
			return tx.writeTree(t)
		},
	}
}

func (tx *Transaction) attributeViewUpdateEnvironment() *attributeViewUpdateEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avUpdate == nil {
			return nil
		}
		e := *tx.environment.avUpdate
		e.ctx, e.legacy = tx.environment.ctx, false
		return &e
	}
	return legacyAttributeViewUpdateEnvironment(tx)
}

func (e *attributeViewUpdateEnvironment) guard(fn func() error) error { return e.r04Core().Guard(fn) }

func transactionUpdateAV(_ context.Context, node *ast.Node, tree *parse.Tree, tx *Transaction) error {
	return updateAttributeViewWithEnvironment(node, tree, tx.attributeViewUpdateEnvironment())
}

func updateAttributeViewWithEnvironment(node *ast.Node, tree *parse.Tree, e *attributeViewUpdateEnvironment) error {
	return editcore.UpdateAttributeViewWithEnvironment(node, tree, e.r04Core())
}

func registerUpdatedAttributeViews(node *ast.Node, e *attributeViewUpdateEnvironment) error {
	return editcore.RegisterUpdatedAttributeViews(node, e.r04Core())
}

func uniqueUpdatedAttributeViews(ids []string) []string {
	return editcore.UniqueUpdatedAttributeViews(ids)
}

func refreshUpdatedAttributeViews(ids []string, e *attributeViewUpdateEnvironment) error {
	return editcore.RefreshUpdatedAttributeViews(ids, e.r04Core())
}

func validateUpdatedAttributeView(a *av.AttributeView) error {
	return editcore.ValidateUpdatedAttributeView(a)
}
