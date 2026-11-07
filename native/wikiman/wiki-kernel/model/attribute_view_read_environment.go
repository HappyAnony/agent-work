// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"strings"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/avquery"
)

// A projection of existing block-query services, not a new source-write seam.
type attributeViewReadEnvironment struct {
	ownerGuard func() error
	strict     bool
	ctx        context.Context
	legacy     bool
	wait       func() error
	read       func(string) (*av.AttributeView, error)
	clone      func(*av.AttributeView) (*av.AttributeView, error)
	mirrors    func(string) ([]string, error)
	exists     func(context.Context, string) (bool, error)
	node       func(string) (*ast.Node, *parse.Tree, error)
	untitled   func() string
}

func legacyAttributeViewReadEnvironment() *attributeViewReadEnvironment {
	return boundAttributeViewReadEnvironment(nil)
}

func boundAttributeViewReadEnvironment(tx *Transaction) *attributeViewReadEnvironment {
	ctx := transactionAVContext(tx)
	binding := legacyAVBindingForTransaction(tx)
	return readWithBinding(tx, binding, ctx)
}
func readWithBinding(tx *Transaction, binding *legacyAVBinding, ctx context.Context) *attributeViewReadEnvironment {
	b := blockKeysWithBinding(tx, binding, ctx)
	return &attributeViewReadEnvironment{ctx: ctx, legacy: true, strict: tx != nil, ownerGuard: b.ownerGuard, wait: b.wait, read: b.read, clone: b.clone, mirrors: b.mirrors, node: b.resolve, untitled: b.untitled,
		exists: func(ctx context.Context, id string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if tx != nil && tx.environment != nil {
				block, err := tx.environment.lookupBlock(id)
				return block != nil, err
			}
			if err := binding.guard(); err != nil {
				return false, err
			}
			block, err := binding.scope.commit.r.referenceLookup(ctx, id)
			return block != nil, err
		},
	}
}

func (tx *Transaction) attributeViewReadEnvironment() *attributeViewReadEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avBlockKeys == nil {
			return nil
		}
		b := *tx.environment.avBlockKeys
		b.ctx, b.legacy = tx.environment.ctx, false
		e := &attributeViewReadEnvironment{ctx: b.ctx, wait: b.wait, read: b.read, clone: b.clone, mirrors: b.mirrors, untitled: b.untitled}
		if b.node != nil {
			e.node = b.resolve
		}
		if tx.environment.lookup != nil {
			e.exists = func(ctx context.Context, id string) (bool, error) {
				if err := ctx.Err(); err != nil {
					return false, err
				}
				block, err := tx.environment.lookupBlock(id)
				return block != nil, err
			}
		}
		return e
	}
	return boundAttributeViewReadEnvironment(tx)
}

func (e *attributeViewReadEnvironment) validate(mode string) error {
	if e == nil || e.ctx == nil || e.wait == nil || e.read == nil || e.clone == nil {
		return errors.New("av_read_dependencies_unbound")
	}
	if mode == "primary" && (e.mirrors == nil || e.exists == nil || e.untitled == nil) {
		return errors.New("av_primary_dependencies_unbound")
	}
	if mode == "rules" && e.node == nil {
		return errors.New("av_rules_dependencies_unbound")
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if e.ownerGuard != nil {
		return e.ownerGuard()
	}
	return nil
}
func (e *attributeViewReadEnvironment) guard(fn func() error) error {
	if e.ownerGuard != nil {
		if err := e.ownerGuard(); err != nil {
			return err
		}
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if e.ownerGuard != nil {
		return e.ownerGuard()
	}
	return nil
}

func readAttributeViewWithEnvironment(id, mode string, e *attributeViewReadEnvironment) (*av.AttributeView, error) {
	if err := e.validate(mode); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, errors.New("invalid_av_read_id")
	}
	if err := e.guard(e.wait); err != nil {
		return nil, err
	}
	var source, a *av.AttributeView
	if err := e.guard(func() (err error) { source, err = e.read(id); return }); err != nil {
		return nil, err
	}
	if source == nil || source.ID != id {
		return nil, errors.New("invalid_av_read_source")
	}
	if err := e.guard(func() (err error) { a, err = e.clone(source); return }); err != nil {
		return nil, err
	}
	if a == nil || a == source || a.ID != id {
		return nil, errors.New("invalid_av_read_clone")
	}
	return a, nil
}

type attributeViewPrimaryResult struct {
	name   string
	blocks []string
	values *av.KeyValues
}

func attributeViewPrimaryWithEnvironment(id, keyword string, page, size int, e *attributeViewReadEnvironment) (ret attributeViewPrimaryResult, err error) {
	defer func() {
		if err != nil && (e == nil || !e.legacy || e.strict) {
			ret = attributeViewPrimaryResult{}
		}
	}()
	a, err := readAttributeViewWithEnvironment(id, "primary", e)
	if err != nil {
		return ret, err
	}
	ret.name = strings.TrimSpace(a.Name)
	if ret.name == "" {
		ret.name = e.untitled()
		if err = e.ctx.Err(); err != nil {
			return ret, err
		}
	}
	if err = e.guard(func() (err error) { ret.blocks, err = e.mirrors(id); return }); err != nil {
		return ret, err
	}
	for _, kv := range a.KeyValues {
		if kv == nil || kv.Key == nil {
			return ret, errors.New("invalid_av_primary_key")
		}
	}
	ret.values = a.GetBlockKeyValues()
	if ret.values == nil {
		return ret, errors.New("av_primary_key_missing")
	}
	ret.values.Values, err = avquery.PrimaryValues(e.ctx, ret.values.Values, func(v *av.Value) (avquery.PrimaryMeta, error) {
		if v == nil || v.Block == nil {
			return avquery.PrimaryMeta{}, errors.New("av_primary_block_missing")
		}
		return avquery.PrimaryMeta{BoundID: v.Block.ID, Text: v.String(true), Updated: v.Block.Updated, Detached: v.IsDetached}, nil
	}, func(ctx context.Context, id string) (ok bool, err error) {
		err = e.guard(func() (err error) { ok, err = e.exists(ctx, id); return })
		return
	}, keyword, page, size)
	if err == nil {
		err = e.ctx.Err()
	}
	return
}

func selectAttributeViewReadView(a *av.AttributeView, blockID string, e *attributeViewReadEnvironment) (*av.View, error) {
	if e == nil || e.ctx == nil || e.node == nil {
		return nil, errors.New("av_rules_dependencies_unbound")
	}
	var selected string
	if blockID != "" {
		var n *ast.Node
		if err := e.guard(func() (err error) { n, _, err = e.node(blockID); return }); err != nil {
			if !e.legacy || e.ctx.Err() != nil || e.strict && !errors.Is(err, ErrBlockNotFound) {
				return nil, err
			}
		} else if n != nil {
			selected = n.IALAttr(av.NodeAttrView)
		}
	}
	return a.GetCurrentView(selected)
}

func attributeViewRulesWithEnvironment(id, blockID string, e *attributeViewReadEnvironment) ([]*av.ViewFilter, []*av.ViewSort, error) {
	a, err := readAttributeViewWithEnvironment(id, "rules", e)
	if err != nil {
		return nil, nil, err
	}
	if err = validateUpdatedAttributeView(a); err != nil {
		return nil, nil, err
	}
	v, err := selectAttributeViewReadView(a, blockID, e)
	if err != nil && (!e.legacy || e.strict) {
		return nil, nil, err
	}
	if v == nil {
		v, err = a.GetCurrentView(a.ViewID)
	}
	if err != nil {
		return nil, nil, err
	}
	if v == nil {
		return nil, nil, errors.New("invalid_av_rules_view")
	}
	filters, sorts := v.Filters, v.Sorts
	if len(filters) == 0 {
		filters = []*av.ViewFilter{}
	}
	if len(sorts) == 0 {
		sorts = []*av.ViewSort{}
	}
	if err = e.ctx.Err(); err != nil {
		return nil, nil, err
	}
	return filters, sorts, nil
}

func (tx *Transaction) readAttributeView(id string) (*av.AttributeView, error) {
	return readAttributeViewWithEnvironment(id, "source", tx.attributeViewReadEnvironment())
}
func (tx *Transaction) attributeViewPrimary(id, keyword string, page, size int) (attributeViewPrimaryResult, error) {
	return attributeViewPrimaryWithEnvironment(id, keyword, page, size, tx.attributeViewReadEnvironment())
}
func (tx *Transaction) attributeViewRules(id, blockID string) ([]*av.ViewFilter, []*av.ViewSort, error) {
	return attributeViewRulesWithEnvironment(id, blockID, tx.attributeViewReadEnvironment())
}
