// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
	// Scoped effects operate on detached owner candidates, never save/broadcast.
)

type attributeViewFieldEnvironment struct {
	strictDependencies bool
	complete           func() error
	legacy             bool
	read               func(string) (*av.AttributeView, error)
	stage              func(*av.AttributeView) error
	groups             func(*av.AttributeView) error
	sources            func(string) ([]string, error)
	removeRelation     func(string, string) error
	reload             func(string) error
}

func isAttributeViewFieldAction(action string) bool {
	return editcore.IsAttributeViewFieldAction(action)
}

func (e *attributeViewFieldEnvironment) validate(action string) error {
	return e.r04Core().Validate(action)
}

func validateAttributeViewFieldInput(op *Operation) error {
	return editcore.ValidateAttributeViewFieldInput(op)
}

// Validate structural dereferences, not conversion policy: original type changes
// retain the old value payload and a newly added relation may be unconfigured.
func validateAttributeViewFieldSource(a *av.AttributeView) error {
	return editcore.ValidateAttributeViewFieldSource(a)
}

func legacyAttributeViewFieldEnvironment(contexts ...context.Context) *attributeViewFieldEnvironment {
	ctx := legacyAVContext(contexts)
	b := newLegacyAVBinding(ctx)
	return fieldWithBinding(b)
}
func fieldWithBinding(b *legacyAVBinding) *attributeViewFieldEnvironment {
	return &attributeViewFieldEnvironment{complete: b.complete, legacy: true, strictDependencies: true, read: b.readCandidate, stage: b.saveUpgraded,
		groups:         b.groups,
		sources:        b.sources,
		removeRelation: b.removeRelation,
		reload:         b.reload}
}

func runAttributeViewField(ctx context.Context, op *Operation, e *attributeViewFieldEnvironment) error {
	return editcore.RunAttributeViewField(ctx, op, e.r04Core())
}

func runLegacyAttributeViewField(op *Operation) error {
	return runAttributeViewField(context.Background(), op, legacyAttributeViewFieldEnvironment())
}
func runLegacyAttributeViewFieldAction(action string, op *Operation) error {
	if op == nil {
		return errors.New("invalid_av_field_operation")
	}
	copy := *op
	copy.Action = action
	return runLegacyAttributeViewField(&copy)
}
func (tx *Transaction) doAttributeViewField(action string, op *Operation) *TxErr {
	if op == nil {
		return &TxErr{code: TxErrHandleAttributeView, msg: "invalid_av_field_operation"}
	}
	copy := *op
	copy.Action = action
	ctx := transactionAVContext(tx)
	var e *attributeViewFieldEnvironment
	if tx.environment != nil && !tx.environment.legacy {
		e = tx.environment.avFields
	} else {
		e = legacyAttributeViewFieldEnvironment(ctx)
	}
	// An injected legacy flag can never opt a scoped transaction into ignored errors.
	if e != nil && tx.environment != nil && !tx.environment.legacy {
		local := *e
		local.legacy = false
		e = &local
	}
	if err := runAttributeViewField(ctx, &copy, e); err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: op.AvID, msg: err.Error()}
	}
	return nil
}
