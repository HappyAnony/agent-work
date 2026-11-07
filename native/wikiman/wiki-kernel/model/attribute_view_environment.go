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
	// All callbacks are owned by the admitted transaction. For scoped execution,
	// stage/index/reload MUST stage effects, not save or publish before R04 commit.
)

type attributeViewEnvironment struct {
	strictDependencies bool
	complete           func() error
	legacy             bool
	read               func(string) (*av.AttributeView, error)
	stage              func(*av.AttributeView) error
	groups             func(*av.AttributeView) error
	removeRelation     func(string, string) error
	upsertRelation     func(string, string) error
	reload             func(string) error
	now                func() time.Time
	newID              func() string
}

func (e *attributeViewEnvironment) validate() error { return e.r04Core().Validate() }

func legacyAttributeViewEnvironment(contexts ...context.Context) *attributeViewEnvironment {
	ctx := legacyAVContext(contexts)
	b := newLegacyAVBinding(ctx)
	return configurationWithBinding(b)
}

func configurationWithBinding(b *legacyAVBinding) *attributeViewEnvironment {
	return &attributeViewEnvironment{complete: b.complete, legacy: true, strictDependencies: true, read: b.readCandidate, stage: b.saveUpgraded, groups: b.groups, removeRelation: b.removeRelation, upsertRelation: b.upsertRelation, reload: b.reload, now: time.Now, newID: ast.NewNodeID}
}
func (tx *Transaction) attributeViewEnvironment() *attributeViewEnvironment {
	if tx == nil || tx.environment == nil || tx.environment.legacy {
		return configurationWithBinding(legacyAVBindingForTransaction(tx))
	}
	return tx.environment.avConfig
}
func runAttributeViewConfiguration(ctx context.Context, op *Operation, e *attributeViewEnvironment, fn func(*Operation, *attributeViewEnvironment) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := e.validate(); err != nil {
		return err
	}
	if op == nil {
		return errors.New("invalid_av_operation")
	}
	switch op.Action {
	case "updateAttrViewColTemplate":
		if _, ok := op.Data.(string); !ok {
			return errors.New("invalid_av_template")
		}
	case "updateAttrViewColRollup":
		if op.Data != nil {
			if _, ok := op.Data.(map[string]interface{}); !ok {
				return errors.New("invalid_av_rollup")
			}
		}
	}
	// Wrap every effect with cancellation. The caller owns discard/atomic finish.
	local := *e
	local.read = func(id string) (*av.AttributeView, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return e.read(id)
	}
	local.stage = func(a *av.AttributeView) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.stage(a)
	}
	local.groups = func(a *av.AttributeView) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.groups(a)
	}
	local.removeRelation = func(a, b string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.removeRelation(a, b)
	}
	local.upsertRelation = func(a, b string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.upsertRelation(a, b)
	}
	local.reload = func(id string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return e.reload(id)
	}
	if err := fn(op, &local); err != nil {
		return err
	}
	if e.complete != nil {
		return e.complete()
	}
	return ctx.Err()
}
func updateAttributeViewColTemplateWithEnvironment(operation *Operation, env *attributeViewEnvironment) (err error) {
	return editcore.UpdateAttributeViewColTemplateWithEnvironment(operation, env.r04Core())
}

func updateAttributeViewColRelationWithEnvironment(operation *Operation, env *attributeViewEnvironment) (err error) {
	return editcore.UpdateAttributeViewColRelationWithEnvironment(operation, env.r04Core())
}

func updateAttributeViewColRollupWithEnvironment(operation *Operation, env *attributeViewEnvironment) (err error) {
	return editcore.UpdateAttributeViewColRollupWithEnvironment(operation, env.r04Core())
}
