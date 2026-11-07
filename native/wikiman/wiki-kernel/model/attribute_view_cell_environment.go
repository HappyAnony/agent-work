// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"

	"fmt"
	"math/rand"

	"time"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
)

// Every scoped callback stages owned candidates/effects. This is not R04 commit.

type attributeViewCellEnvironment struct {
	strictDependencies bool
	complete           func() error
	legacy             bool
	ctx                context.Context
	read               func(string) (*av.AttributeView, error)
	stage              func(*av.AttributeView) error
	groups             func(*av.AttributeView) error
	node               func(string) (*ast.Node, *parse.Tree, error)
	blockText          func(*ast.Node) (string, error)
	names              func(string) (string, error)
	attrs              func(*ast.Node, *parse.Tree, map[string]string) error
	related            func(string) error
	now                func() time.Time
	newID              func() string
	color              func() string
}

func (e *attributeViewCellEnvironment) validate() error { return e.r04Core().Validate() }

func (tx *Transaction) attributeViewCellEnvironment() *attributeViewCellEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avCell == nil {
			return nil
		}
		e := *tx.environment.avCell
		e.legacy = false
		e.ctx = tx.environment.ctx
		return &e
	}
	return legacyAttributeViewCellWithBinding(tx, legacyAVBindingForTransaction(tx))
}
func legacyAttributeViewCellWithBinding(tx *Transaction, b *legacyAVBinding) *attributeViewCellEnvironment {
	ctx := transactionAVContext(tx)
	if tx == nil && b.err == nil {
		ctx = b.scope.ctx
	}
	return &attributeViewCellEnvironment{legacy: true, strictDependencies: true, complete: b.complete, ctx: ctx, read: b.readCandidate, stage: b.saveUpgraded,
		groups: b.groups, node: func(id string) (*ast.Node, *parse.Tree, error) { return b.node(tx, id) },
		blockText: func(n *ast.Node) (string, error) { _, text, err := b.text(n, ""); return text, err }, names: b.names,
		attrs:   func(n *ast.Node, t *parse.Tree, attrs map[string]string) error { return b.attributes(tx, n, t, attrs) },
		related: func(id string) error { return b.related(id, tx) }, now: time.Now, newID: ast.NewNodeID, color: func() string { return fmt.Sprintf("%d", 1+rand.Intn(14)) },
	}
}

// Preserve original binding/attribute algorithms for both adapters; legacy
// wrappers keep best-effort behavior, scoped callers must propagate errors.
func cellBindingAttrs(avID string, n *ast.Node, bind bool, names func(string) (string, error)) (map[string]string, error) {
	return editcore.CellBindingAttrs(avID, n, bind, names)
}

func (e *attributeViewCellEnvironment) binding(avID, blockID string, bind bool) error {
	return e.r04Core().Binding(avID, blockID, bind)
}

func (e *attributeViewCellEnvironment) staticText(n *ast.Node, t *parse.Tree, avID, text string) error {
	return e.r04Core().StaticText(n, t, avID, text)
}

func validateCellValue(a *av.AttributeView, keyID, itemID string, data interface{}) error {
	return editcore.ValidateCellValue(a, keyID, itemID, data)
}

func validateCellSource(a *av.AttributeView) error { return editcore.ValidateCellSource(a) }

func collectRelatedAttributeViews(ctx context.Context, id string, sources func(string) ([]string, error), enqueue func([]string) error) error {
	return editcore.CollectRelatedAttributeViews(ctx, id, sources, enqueue)
}

// Original finish order: read, regenerate, stage, reload. Scoped reload is a
// candidate event sink, not a WebSocket publication. Legacy keeps best effort.
func flushRelatedAttributeViews(ctx context.Context, ids []string, read func(string) (*av.AttributeView, error), groups func(*av.AttributeView) error, stage func(*av.AttributeView) error, reload func(string) error, legacy bool) error {
	return editcore.FlushRelatedAttributeViews(ctx, ids, read, groups, stage, reload, legacy)
}
