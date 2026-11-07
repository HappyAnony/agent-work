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
	// Candidate-only owner effects. R04 supplies authoritative commit/publication.
)

type attributeViewNameEnvironment struct {
	ownerGuard         func() error
	strictDependencies bool
	complete           func() error
	names              func(string) (string, error)
	ctx                context.Context
	legacy             bool
	read               func(string) (*av.AttributeView, error)
	stage              func(*av.AttributeView) error
	exists             func(string) (bool, error)
	node               func(string) (*ast.Node, *parse.Tree, error)
	name               func(string) (string, error)
	untitled           func() string
	tree               func(*parse.Tree) error
	cache              func(string, map[string]string) error
	notify             func(map[string]string, *ast.Node) error
}

func validateAttributeViewNameInput(op *Operation) error {
	return editcore.ValidateAttributeViewNameInput(op)
}

func (e *attributeViewNameEnvironment) validate() error { return e.r04Core().Validate() }

func (tx *Transaction) attributeViewNameEnvironment() *attributeViewNameEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avName == nil {
			return nil
		}
		e := *tx.environment.avName
		e.ctx = tx.environment.ctx
		e.legacy = false
		return &e
	}
	b := legacyAVBindingForTransaction(tx)
	publish := cache.CaptureBlockIALPublisher()
	return &attributeViewNameEnvironment{ctx: transactionAVContext(tx), legacy: true, strictDependencies: true, ownerGuard: b.guard, complete: b.complete,
		read: b.readCandidate, stage: b.saveUpgraded, names: b.names,
		exists: func(id string) (bool, error) { return b.blockExistsFor(tx, id) }, node: func(id string) (*ast.Node, *parse.Tree, error) { return b.node(tx, id) },
		name: func(id string) (string, error) {
			a, e := b.readCandidate(id)
			if e != nil {
				return "", e
			}
			return a.Name, nil
		},
		untitled: func() string {
			if b.err != nil {
				return ""
			}
			return b.scope.commit.avUntitled
		},
		// Original tx node loads register candidates for commitLegacy; do not introduce
		// general attrs/tag/anchor effects or persist the derived av-names field.
		tree: func(*parse.Tree) error { return b.guard() }, cache: func(id string, attrs map[string]string) error {
			return b.effect(func() error { publish(id, attrs); return nil })
		},
		notify: func(old map[string]string, n *ast.Node) error {
			return b.effect(func() error { pushBroadcastAttrTransactions(old, n); return nil })
		}}
}

func runAttributeViewName(op *Operation, e *attributeViewNameEnvironment) error {
	return editcore.RunAttributeViewName(op, e.r04Core())
}
