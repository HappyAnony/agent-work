// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"

	"time"
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
)

var errInvalidAVOrderSource = editcore.ErrInvalidAVOrderSource

// All scoped effects are candidate-only; R04 owns source commit/publication.
type attributeViewOrderEnvironment struct {
	complete func() error
	read     func(string) (*av.AttributeView, error)
	stage    func(*av.AttributeView) error
	view     func(*av.AttributeView, string) (*av.View, error)
	groups   func(*av.AttributeView) error
	cloneKey func(*av.Key) (*av.Key, error)
	defaults *attributeViewDefaultEnvironment
}
type attributeViewDefaultEnvironment struct {
	ctx          context.Context
	near         func(*av.AttributeView, *av.View, *av.View, string) (av.Item, error)
	templateKeys func(*av.AttributeView, *av.Key) ([]*av.Key, error)
	reverse      func(*av.AttributeView, *av.Key, *av.Value) error
	now          func() time.Time
	newID        func() string
}

func (e *attributeViewDefaultEnvironment) validate() error { return e.r04Core().Validate() }

func (e *attributeViewDefaultEnvironment) nearValue(item av.Item, key *av.Key, row string) *av.Value {
	return e.r04Core().NearValue(item, key, row)
}

func selectAttributeViewNearItem(c av.Collection, previous string) av.Item {
	return editcore.SelectAttributeViewNearItem(c, previous)
}

func cloneAttributeViewOrderKey(key *av.Key) (*av.Key, error) {
	return editcore.CloneAttributeViewOrderKey(key)
}

func legacyAttributeViewDefaults(contexts ...context.Context) *attributeViewDefaultEnvironment {
	ctx := legacyAVContext(contexts)
	return defaultsWithBinding(newLegacyAVBinding(ctx), ctx)
}
func defaultsWithBinding(b *legacyAVBinding, ctx context.Context) *attributeViewDefaultEnvironment {
	return &attributeViewDefaultEnvironment{ctx: ctx, now: time.Now, newID: ast.NewNodeID,
		near: func(a *av.AttributeView, v, g *av.View, p string) (av.Item, error) {
			return b.near(a, v, g, p)
		},
		templateKeys: func(a *av.AttributeView, k *av.Key) ([]*av.Key, error) {
			return b.templateKeys(a, k)
		},
		reverse: func(a *av.AttributeView, k *av.Key, v *av.Value) error {
			return updateTwoWayRelationDestWithEnvironment(legacyAttributeViewCellWithBinding(nil, b), a, k, v, 1, []string{})
		}}
}
func legacyAttributeViewOrderEnvironment(contexts ...context.Context) *attributeViewOrderEnvironment {
	ctx := legacyAVContext(contexts)
	b := newLegacyAVBinding(ctx)
	return orderWithBinding(b, ctx)
}
func orderWithBinding(b *legacyAVBinding, ctx context.Context) *attributeViewOrderEnvironment {
	e := settingsWithBinding(b)
	return &attributeViewOrderEnvironment{complete: b.complete, read: e.read, stage: e.stage, view: e.view, cloneKey: cloneAttributeViewOrderKey, groups: b.groups, defaults: defaultsWithBinding(b, ctx)}
}
func isAttributeViewOrderAction(s string) bool { return editcore.IsAttributeViewOrderAction(s) }

func (e *attributeViewOrderEnvironment) validate(action string) error {
	return e.r04Core().Validate(action)
}

func validateAttributeViewOrderInput(op *Operation) error {
	return editcore.ValidateAttributeViewOrderInput(op)
}

func runAttributeViewOrder(ctx context.Context, op *Operation, e *attributeViewOrderEnvironment) error {
	return editcore.RunAttributeViewOrder(ctx, op, e.r04Core())
}

func runLegacyAttributeViewOrder(action string, op *Operation) error {
	if op == nil {
		return errors.New("invalid_av_order_operation")
	}
	copy := *op
	copy.Action = action
	return runAttributeViewOrder(context.Background(), &copy, legacyAttributeViewOrderEnvironment())
}
func (tx *Transaction) doAttributeViewOrder(action string, op *Operation) *TxErr {
	if op == nil {
		return &TxErr{code: TxErrHandleAttributeView, msg: "invalid_av_order_operation"}
	}
	copy := *op
	copy.Action = action
	ctx := transactionAVContext(tx)
	var e *attributeViewOrderEnvironment
	if tx.environment != nil && !tx.environment.legacy {
		e = tx.environment.avOrder
	} else {
		e = legacyAttributeViewOrderEnvironment(ctx)
	}
	if err := runAttributeViewOrder(ctx, &copy, e); err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: op.AvID, msg: err.Error()}
	}
	return nil
}
