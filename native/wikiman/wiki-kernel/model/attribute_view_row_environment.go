// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"

	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"wiki-kernel/av"
	"wiki-kernel/internal/editcore"
	"wiki-kernel/util"
	// Scoped effects are owned candidates. Publication remains an R04 responsibility.
)

type attributeViewRowEnvironment struct {
	cell         *attributeViewCellEnvironment
	defaults     *attributeViewDefaultEnvironment
	view         func(*av.AttributeView, string) (*av.View, error)
	text         func(*ast.Node) (string, string, error)
	optionalNode func(string) (*ast.Node, *parse.Tree, error)
	exists       func(string) (bool, error)
	duplicate    func(*av.AttributeView) error
	history      func(*av.AttributeView) error
}

func isAttributeViewRowAction(s string) bool { return editcore.IsAttributeViewRowAction(s) }

func validateAttributeViewRowInput(op *Operation) error {
	return editcore.ValidateAttributeViewRowInput(op)
}

func (e *attributeViewRowEnvironment) validate(action string) error {
	return e.r04Core().Validate(action)
}

func (tx *Transaction) attributeViewRowEnvironment() *attributeViewRowEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avRows == nil {
			return nil
		}
		e := *tx.environment.avRows
		if e.cell != nil {
			c := *e.cell
			c.ctx = tx.environment.ctx
			c.legacy = false
			e.cell = &c
		}
		if e.defaults != nil {
			d := *e.defaults
			d.ctx = tx.environment.ctx
			e.defaults = &d
		}
		return &e
	}
	b := legacyAVBindingForTransaction(tx)
	return rowWithBinding(tx, b, transactionAVContext(tx))
}
func rowWithBinding(tx *Transaction, b *legacyAVBinding, ctx context.Context) *attributeViewRowEnvironment {
	c := legacyAttributeViewCellWithBinding(tx, b)
	return &attributeViewRowEnvironment{cell: c, defaults: defaultsWithBinding(b, ctx), view: b.readView,
		text: func(n *ast.Node) (string, string, error) { return b.text(n, "") },
		optionalNode: func(id string) (*ast.Node, *parse.Tree, error) {
			yes, err := b.blockExistsFor(tx, id)
			if err != nil {
				return nil, nil, err
			}
			if !yes {
				return nil, nil, nil
			}
			n, t, e := c.node(id)
			return n, t, e
		},
		exists: b.existsAV,
		duplicate: func(a *av.AttributeView) error {
			return b.effect(func() error {
				name := strings.TrimSpace(a.Name)
				if name == "" {
					name = b.scope.commit.avUntitled
				}
				util.PushMsg(fmt.Sprintf(b.scope.commit.avLanguage(269), name), 5000)
				return nil
			})
		}, history: func(a *av.AttributeView) error {
			return b.effect(func() error { return legacyAttributeViewRowHistory(a) })
		}}
}

func runAttributeViewRow(op *Operation, e *attributeViewRowEnvironment) error {
	return editcore.RunAttributeViewRow(op, e.r04Core())
}

func batchReplaceAttributeViewBlocksWithEnvironment(id string, detached bool, batch []map[string]string, e *attributeViewRowEnvironment) error {
	// Validate the entire batch before reading any source or staging a binding.
	for _, m := range batch {
		for old, next := range m {
			if err := validateAttributeViewRowInput(&Operation{Action: "replaceAttrViewBlock", AvID: id, PreviousID: old, NextID: next, IsDetached: detached}); err != nil {
				return err
			}
		}
	}
	if err := e.validate("replaceAttrViewBlock"); err != nil {
		return err
	}
	if err := e.cell.ctx.Err(); err != nil {
		return err
	}
	a, err := e.cell.read(id)
	if err != nil {
		return err
	}
	if err = validateCellSource(a); err != nil {
		return err
	}
	local := *e
	c := *e.cell
	c.complete = nil
	local.cell = &c
	c.read = func(request string) (*av.AttributeView, error) {
		if request == id {
			return a, nil
		}
		return e.cell.read(request)
	}
	c.stage = func(candidate *av.AttributeView) error { a = candidate; return e.cell.ctx.Err() }
	for _, m := range batch {
		for old, next := range m {
			if err = e.cell.ctx.Err(); err != nil {
				return err
			}
			if err = runAttributeViewRow(&Operation{Action: "replaceAttrViewBlock", AvID: id, PreviousID: old, NextID: next, IsDetached: detached}, &local); err != nil {
				return err
			}
		}
	}
	if err = e.cell.ctx.Err(); err != nil {
		return err
	}
	if err = e.cell.stage(a); err != nil {
		return err
	}
	if e.cell.complete != nil {
		return e.cell.complete()
	}
	return e.cell.ctx.Err()
}
