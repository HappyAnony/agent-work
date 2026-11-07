// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
	// Scoped adapters own detached candidates; they must not Save/broadcast early.
)

type attributeViewOptionEnvironment struct {
	complete func() error
	read     func(string) (*av.AttributeView, error)
	stage    func(*av.AttributeView) error
	groups   func(*av.AttributeView) error
}

func isAttributeViewOptionAction(action string) bool {
	switch action {
	case "updateAttrViewColOptions", "removeAttrViewColOption", "updateAttrViewColOption", "setAttrViewColOptionDesc":
		return true
	}
	return false
}

func (e *attributeViewOptionEnvironment) validate(action string) error {
	return e.r04Core().Validate(action)
}

func validateAttributeViewOptionInput(op *Operation) error {
	return editcore.ValidateAttributeViewOptionInput(op)
}

func validateAttributeViewOptionSource(a *av.AttributeView, op *Operation) error {
	return editcore.ValidateAttributeViewOptionSource(a, op)
}

func legacyAttributeViewOptionEnvironment(contexts ...context.Context) *attributeViewOptionEnvironment {
	ctx := legacyAVContext(contexts)
	b := newLegacyAVBinding(ctx)
	return &attributeViewOptionEnvironment{complete: b.complete, read: b.readCandidate, stage: b.saveUpgraded, groups: b.groups}
}

func runAttributeViewOption(ctx context.Context, op *Operation, e *attributeViewOptionEnvironment) error {
	return editcore.RunAttributeViewOption(ctx, op, e.r04Core())
}

func runLegacyAttributeViewOption(action string, op *Operation) error {
	if op == nil {
		return errors.New("invalid_av_option_operation")
	}
	copy := *op
	copy.Action = action
	return runAttributeViewOption(context.Background(), &copy, legacyAttributeViewOptionEnvironment())
}

func (tx *Transaction) doAttributeViewOption(op *Operation) *TxErr {
	ctx := transactionAVContext(tx)
	var e *attributeViewOptionEnvironment
	if tx.environment != nil && !tx.environment.legacy {
		e = tx.environment.avOptions
	} else {
		e = legacyAttributeViewOptionEnvironment(ctx)
	}
	if err := runAttributeViewOption(ctx, op, e); err != nil {
		return &TxErr{code: TxErrHandleAttributeView, id: op.AvID, msg: err.Error()}
	}
	return nil
}
