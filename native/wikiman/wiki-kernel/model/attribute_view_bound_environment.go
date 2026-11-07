// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/cache"
	"wiki-kernel/internal/editcore"
	// Transaction binding effects are candidates in an owned adapter. R04 supplies
	// the authoritative source commit, history and event publication boundary.
)

type attributeViewBoundEnvironment struct {
	ownerGuard         func() error
	strictDependencies bool
	complete           func() error
	ctx                context.Context
	legacy             bool
	read               func(string) (*av.AttributeView, error)
	stage              func(*av.AttributeView) error
	groups             func(*av.AttributeView) error
	exists             func(string) (bool, error)
	load               func(string) (*parse.Tree, error)
	isMirror           func(string) (bool, error)
	removeMirror       func(string, string) (bool, error)
	names              func(string) (string, error)
	attrs              func(*ast.Node, *parse.Tree, map[string]string) error
	cache              func(string, map[string]string) error
	notify             func(map[string]string, *ast.Node) error
	tree               func(*parse.Tree) error
	reload             func(string) error
}

func (e *attributeViewBoundEnvironment) validate(mode string) error {
	return e.r04Core().Validate(mode)
}

func (tx *Transaction) attributeViewBoundEnvironment() *attributeViewBoundEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avBound == nil {
			return nil
		}
		e := *tx.environment.avBound
		e.ctx, e.legacy = tx.environment.ctx, false
		return &e
	}
	b := legacyAVBindingForTransaction(tx)
	ctx := context.Background()
	if tx != nil && tx.environment != nil {
		ctx = tx.environment.ctx
	}
	return &attributeViewBoundEnvironment{ctx: ctx, legacy: true, strictDependencies: true, ownerGuard: b.guard, complete: b.complete, read: b.readCandidate, stage: b.saveUpgraded,
		groups: b.groups,
		exists: func(id string) (bool, error) { return b.blockExistsFor(tx, id) },
		load: func(id string) (*parse.Tree, error) {
			return b.tree(tx, id)
		},
		isMirror: b.isMirror,
		removeMirror: func(id, block string) (bool, error) {
			return b.removeMirror(tx, id, block)
		},
		names: b.names,
		attrs: func(n *ast.Node, t *parse.Tree, attrs map[string]string) error {
			return b.attributes(tx, n, t, attrs)
		}, cache: func(id string, attrs map[string]string) error { cache.PutBlockIAL(id, attrs); return nil },
		notify: func(old map[string]string, n *ast.Node) error { pushBroadcastAttrTransactions(old, n); return nil },
		tree: func(t *parse.Tree) error {
			if tx == nil {
				return indexWriteTreeUpsertQueue(t)
			}
			return tx.writeTree(t)
		},
		reload: b.reload,
	}
}

func (e *attributeViewBoundEnvironment) guard(fn func() error) error { return e.r04Core().Guard(fn) }

func collectAttributeViewBoundNodes(a *av.AttributeView, e *attributeViewBoundEnvironment) (map[string]*parse.Tree, []*ast.Node, error) {
	return editcore.CollectAttributeViewBoundNodes(a, e.r04Core())
}

// Also used by the >128-operation paste path; it must not bypass owner ports.
func transactionInsertAVBind(_ context.Context, n *ast.Node, tree *parse.Tree, tx *Transaction) error {
	return editcore.InsertAttributeViewBindings(n, tree, tx.attributeViewBoundEnvironment().r04Core())
}

func deleteAttributeViewBindings(node *ast.Node, nodeTree *parse.Tree, e *attributeViewBoundEnvironment) error {
	return editcore.DeleteAttributeViewBindings(node, nodeTree, e.r04Core())
}
