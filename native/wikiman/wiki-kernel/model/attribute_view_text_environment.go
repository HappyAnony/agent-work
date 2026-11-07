// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"

	"github.com/88250/lute/ast"
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
)

type attributeViewTextEnvironment struct {
	ctx        context.Context
	jobContext context.Context
	legacy     bool
	// read includes this owner’s staged candidates, including related-field refreshes.
	read     func(string) (*av.AttributeView, error)
	clone    func(*av.AttributeView) (*av.AttributeView, error)
	text     func(*ast.Node, string) (string, string, error)
	stage    func(*av.AttributeView) error
	reload   func(string) error
	related  func(string) error
	complete func() error
}

func (e *attributeViewTextEnvironment) validate() error { return e.r04Core().Validate() }

func (e *attributeViewTextEnvironment) guard(fn func() error) error { return e.r04Core().Guard(fn) }

func legacyAttributeViewTextEnvironment() *attributeViewTextEnvironment {
	return legacyAttributeViewTextEnvironmentContext(context.Background())
}
func legacyAttributeViewTextEnvironmentContext(ctx context.Context) *attributeViewTextEnvironment {
	return legacyAVTextEnvironment(ctx, newLegacyAVBinding(ctx))
}
func legacyAVTextEnvironment(ctx context.Context, b *legacyAVBinding) *attributeViewTextEnvironment {
	return &attributeViewTextEnvironment{ctx: ctx, read: b.read, clone: b.clone, text: b.text, stage: b.save, reload: b.reload, related: func(id string) error { return b.related(id, nil) }, complete: b.complete}
}

func (tx *Transaction) attributeViewTextEnvironment() *attributeViewTextEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avText == nil {
			return nil
		}
		e := *tx.environment.avText
		e.ctx, e.legacy = tx.environment.ctx, false
		return &e
	}
	return legacyAVTextEnvironment(transactionAVContext(tx), legacyAVBindingForTransaction(tx))
}
func (tx *Transaction) updateAttributeViewBlockText(nodes map[string]*ast.Node) error {
	return updateAttributeViewBlockTextWithEnvironment(nodes, tx.attributeViewTextEnvironment())
}

// Native writes/notifications are candidates; the source commit remains R04.
func updateAttributeViewBlockTextWithEnvironment(nodes map[string]*ast.Node, e *attributeViewTextEnvironment) error {
	return editcore.UpdateAttributeViewBlockTextWithEnvironment(nodes, e.r04Core())
}
