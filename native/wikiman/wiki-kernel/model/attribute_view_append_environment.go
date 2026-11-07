// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"time"
	"wiki-kernel/av"
)

// Standalone append seam; it does not add a performTx action or source commit.
type attributeViewAppendEnvironment struct {
	complete   func() error
	ctx        context.Context
	read       func(string) (*av.AttributeView, error)
	stage      func(*av.AttributeView) error
	groups     func(*av.AttributeView) error
	reload     func(string) error
	now        func() time.Time
	newID      func() string
	cloneValue func(*av.Value) (*av.Value, error)
}

func (e *attributeViewAppendEnvironment) validate() error {
	if e == nil || e.ctx == nil || e.read == nil || e.stage == nil || e.groups == nil || e.reload == nil || e.now == nil || e.newID == nil || e.cloneValue == nil {
		return errors.New("av_append_dependencies_unbound")
	}
	return e.ctx.Err()
}
func legacyAttributeViewAppendEnvironment(contexts ...context.Context) *attributeViewAppendEnvironment {
	ctx := legacyAVContext(contexts)
	b := newLegacyAVBinding(ctx)
	return appendWithBinding(b, ctx)
}

func appendWithBinding(b *legacyAVBinding, ctx context.Context) *attributeViewAppendEnvironment {
	return &attributeViewAppendEnvironment{complete: b.complete, ctx: ctx, read: b.readCandidate, stage: b.saveUpgraded, groups: b.groups, reload: b.reload, now: time.Now, newID: ast.NewNodeID, cloneValue: cloneAttributeViewAppendValue}
}
func cloneAttributeViewAppendValue(v *av.Value) (*av.Value, error) {
	copy := v.Clone()
	if copy == nil {
		return nil, errors.New("invalid_av_append_value_clone")
	}
	return copy, nil
}
func (tx *Transaction) attributeViewAppendEnvironment() *attributeViewAppendEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avAppend == nil {
			return nil
		}
		e := *tx.environment.avAppend
		e.ctx = tx.environment.ctx
		return &e
	}
	return appendWithBinding(legacyAVBindingForTransaction(tx), transactionAVContext(tx))
}
func (tx *Transaction) appendAttributeViewDetachedBlocksWithValues(id string, rows [][]*av.Value) error {
	return appendAttributeViewDetachedBlocksWithEnvironment(id, rows, tx.attributeViewAppendEnvironment())
}

func appendAttributeViewDetachedBlocksWithEnvironment(id string, rows [][]*av.Value, e *attributeViewAppendEnvironment) error {
	if id == "" {
		return errors.New("invalid_av_append_id")
	}
	if err := e.validate(); err != nil {
		return err
	}
	ctx := e.ctx
	guard := func(fn func() error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			return err
		}
		return ctx.Err()
	}
	var a *av.AttributeView
	if err := guard(func() (err error) { a, err = e.read(id); return }); err != nil {
		return err
	}
	if err := validateCellSource(a); err != nil {
		return err
	}
	if err := validateAttributeViewFieldSource(a); err != nil {
		return err
	}
	if a.ID != id || a.GetBlockKeyValues() == nil {
		return errors.New("invalid_av_append_source")
	}
	for _, v := range a.Views {
		if !validAttributeViewManagementLayout(v.LayoutType) {
			return av.ErrWrongLayoutType
		}
		if (v.Table != nil && v.Table.BaseLayout == nil) || (v.Gallery != nil && v.Gallery.BaseLayout == nil) || (v.Kanban != nil && v.Kanban.BaseLayout == nil) {
			return errors.New("invalid_av_append_layout")
		}
		for _, g := range v.Groups {
			if g == nil {
				return errors.New("invalid_av_append_group")
			}
		}
	}
	// Validate the entire batch before allocating IDs or mutating the candidate.
	for _, row := range rows {
		for _, v := range row {
			if v == nil {
				return errors.New("invalid_av_append_value")
			}
			kv, err := a.GetKeyValues(v.KeyID)
			if err != nil || kv == nil {
				return fmt.Errorf("key [%s] not found", v.KeyID)
			}
			if kv.Key.Type == av.KeyTypeBlock && v.Block == nil {
				return errors.New("invalid_av_append_block")
			}
			if kv.Key.Type == av.KeyTypeSelect || kv.Key.Type == av.KeyTypeMSelect {
				for _, opt := range v.MSelect {
					if opt == nil {
						return errors.New("invalid_av_append_option")
					}
				}
			}
		}
	}
	prepared := make([][]*av.Value, len(rows))
	for i, row := range rows {
		for _, v := range row {
			var copy *av.Value
			if err := guard(func() (err error) { copy, err = e.cloneValue(v); return }); err != nil {
				return err
			}
			if copy == nil || copy == v || copy.KeyID != v.KeyID {
				return errors.New("invalid_av_append_value_clone")
			}
			kv, _ := a.GetKeyValues(copy.KeyID)
			if kv.Key.Type == av.KeyTypeBlock && copy.Block == nil {
				return errors.New("invalid_av_append_block")
			}
			for _, opt := range copy.MSelect {
				if opt == nil {
					return errors.New("invalid_av_append_option")
				}
			}
			prepared[i] = append(prepared[i], copy)
		}
	}
	used := map[string]bool{}
	for _, kv := range a.KeyValues {
		for _, v := range kv.Values {
			used[v.ID] = true
			used[v.BlockID] = true
		}
	}
	fresh := func() (id string, err error) {
		err = guard(func() error {
			id = e.newID()
			if id == "" || used[id] {
				return errors.New("invalid_av_append_generated_id")
			}
			used[id] = true
			return nil
		})
		return
	}
	var now int64
	if err := guard(func() error { now = e.now().UnixMilli(); return nil }); err != nil {
		return err
	}
	var rowIDs []string
	for _, row := range prepared {
		rowID, err := fresh()
		if err != nil {
			return err
		}
		rowIDs = append(rowIDs, rowID)
		for _, v := range row {
			kv, _ := a.GetKeyValues(v.KeyID)
			v.ID, err = fresh()
			if err != nil {
				return err
			}
			v.BlockID = rowID
			v.Type = kv.Key.Type
			if v.Type == av.KeyTypeBlock {
				v.Block.Created = now
				v.Block.Updated = now
				v.Block.ID = ""
			}
			v.IsDetached = true
			v.CreatedAt = now
			v.UpdatedAt = now
			v.IsRenderAutoFill = false
			kv.Values = append(kv.Values, v)
			if v.Type == av.KeyTypeSelect || v.Type == av.KeyTypeMSelect {
				for _, opt := range v.MSelect {
					if known := kv.Key.GetOption(opt.Content); known == nil {
						kv.Key.Options = append(kv.Key.Options, &av.SelectOption{Name: opt.Content, Color: opt.Color})
					} else {
						opt.Color = known.Color
					}
				}
			}
		}
	}
	for _, v := range a.Views {
		v.ItemIDs = append(v.ItemIDs, rowIDs...)
	}
	a.RenderedViewables = map[string]av.Viewable{}
	if err := guard(func() error { return e.groups(a) }); err != nil {
		return err
	}
	if err := guard(func() error { return e.stage(a) }); err != nil {
		return err
	}
	if err := guard(func() error { return e.reload(id) }); err != nil {
		return err
	}
	if e.complete != nil {
		return e.complete()
	}
	return ctx.Err()
}
