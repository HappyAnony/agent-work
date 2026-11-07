// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"time"

	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"wiki-kernel/av"
	"wiki-kernel/internal/avquery"
)

// Read-only preview: no save, reverse relation, registration or publication port.
type attributeViewPreviewEnvironment struct {
	ctx          context.Context
	read         func(string) (*av.AttributeView, error)
	clone        func(*av.AttributeView) (*av.AttributeView, error)
	near         func(*av.AttributeView, *av.View, *av.View, string) (av.Item, error)
	templateKeys func(*av.AttributeView, *av.Key) ([]*av.Key, error)
	now          func() time.Time
	newID        func() string
}

func (e *attributeViewPreviewEnvironment) validate() error {
	if e == nil || e.ctx == nil || e.read == nil || e.clone == nil || e.near == nil || e.templateKeys == nil || e.now == nil || e.newID == nil {
		return errors.New("av_preview_dependencies_unbound")
	}
	return e.ctx.Err()
}

func cloneAttributeViewPreview(a *av.AttributeView) (*av.AttributeView, error) {
	raw, err := gulu.JSON.MarshalJSON(a)
	if err != nil {
		return nil, err
	}
	copy := &av.AttributeView{}
	if err = gulu.JSON.UnmarshalJSON(raw, copy); err != nil {
		return nil, err
	}
	// Unlike AttributeView.Clone, a preview must retain all source identities and
	// must not allocate IDs or inherit a stale/global runtime renderer or resolver.
	copy.RenderedViewables = map[string]av.Viewable{}
	return copy, nil
}

func legacyAttributeViewPreviewEnvironment(contexts ...context.Context) *attributeViewPreviewEnvironment {
	ctx := legacyAVContext(contexts)
	return previewWithBinding(readonlyAVBinding(nil, ctx), ctx)
}
func previewWithBinding(b *legacyAVBinding, ctx context.Context) *attributeViewPreviewEnvironment {
	return &attributeViewPreviewEnvironment{ctx: ctx, read: b.readCandidate, clone: b.clone, near: b.near, templateKeys: b.templateKeys, now: func() time.Time {
		if b.err != nil {
			return time.Time{}
		}
		return b.scope.now
	}, newID: ast.NewNodeID}
}
func (tx *Transaction) attributeViewPreviewEnvironment() *attributeViewPreviewEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avPreview == nil {
			return nil
		}
		e := *tx.environment.avPreview
		e.ctx = tx.environment.ctx
		return &e
	}
	ctx := transactionAVContext(tx)
	return previewWithBinding(readonlyAVBinding(tx, ctx), ctx)
}

func (tx *Transaction) attrViewAddingBlockDefaultValues(avID, viewID, groupID, previousID, addingID string) (map[string]*av.Value, error) {
	return attrViewAddingBlockDefaultValuesWithEnvironment(avID, viewID, groupID, previousID, addingID, tx.attributeViewPreviewEnvironment())
}

func attrViewAddingBlockDefaultValuesWithEnvironment(avID, viewID, groupID, previousID, addingID string, e *attributeViewPreviewEnvironment) (map[string]*av.Value, error) {
	if avID == "" {
		return nil, errors.New("invalid_av_preview_id")
	}
	if err := e.validate(); err != nil {
		return nil, err
	}
	var pending error
	check := func() error {
		if pending != nil {
			return pending
		}
		return e.ctx.Err()
	}
	guard := func(fn func() error) error {
		if err := check(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			return err
		}
		return check()
	}
	var source, a *av.AttributeView
	if err := guard(func() (err error) { source, err = e.read(avID); return }); err != nil {
		return nil, err
	}
	if err := validateCellSource(source); err != nil {
		return nil, err
	}
	if err := validateAttributeViewFieldSource(source); err != nil {
		return nil, err
	}
	if source.ID != avID {
		return nil, errors.New("invalid_av_preview_source")
	}
	if err := guard(func() (err error) { a, err = e.clone(source); return }); err != nil {
		return nil, err
	}
	if a == nil || a == source || a.ID != avID {
		return nil, errors.New("invalid_av_preview_clone")
	}
	if err := validateCellSource(a); err != nil {
		return nil, err
	}
	if err := validateAttributeViewFieldSource(a); err != nil {
		return nil, err
	}
	a.RenderedViewables = map[string]av.Viewable{}
	view, err := avquery.CurrentView(e.ctx, a.Views, viewID, a.ViewID, func(v *av.View) string { return v.ID })
	if err != nil {
		return nil, err
	}
	// Retain the original early return before resolving a stale group ID.
	if len(view.Filters) == 0 && !view.IsGroupView() {
		return map[string]*av.Value{}, nil
	}
	group := view
	for _, g := range view.Groups {
		if g == nil {
			return nil, errInvalidAVOrderSource
		}
	}
	if groupID != "" {
		group = view.GetGroupByID(groupID)
	}
	if group == nil {
		return nil, errors.New("av_preview_group_not_found")
	}
	// The renderer reads layout settings and typed values; reject malformed
	// payloads before delegating instead of returning a recovered panic.
	if err = validateAttributeViewPreviewLayout(view); err != nil {
		return nil, err
	}
	if err = validateAttributeViewPreviewLayout(group); err != nil {
		return nil, err
	}
	if view.IsGroupView() && view.GetGroupKey(a) == nil {
		return nil, errInvalidAVOrderSource
	}
	for _, filter := range view.Filters {
		if filter == nil || !validAttributeViewDefaultFilter(filter) || (filter.Value != nil && filter.Value.Type == av.KeyTypeDate && filter.Value.Date == nil) {
			return nil, errInvalidAVOrderSource
		}
	}
	for _, sort := range view.Sorts {
		if sort == nil {
			return nil, errInvalidAVOrderSource
		}
	}
	if group.GroupVal != nil && (!validAttributeViewDefaultFilter(&av.ViewFilter{Value: group.GroupVal}) || (group.GroupVal.Type == av.KeyTypeDate && group.GroupVal.Date == nil)) {
		return nil, errInvalidAVOrderSource
	}
	ids := map[string]bool{}
	d := &attributeViewDefaultEnvironment{ctx: e.ctx,
		near: func(a *av.AttributeView, v, g *av.View, p string) (item av.Item, err error) {
			err = guard(func() (err error) { item, err = e.near(a, v, g, p); return })
			return
		}, templateKeys: func(a *av.AttributeView, key *av.Key) (keys []*av.Key, err error) {
			err = guard(func() (err error) { keys, err = e.templateKeys(a, key); return })
			if err == nil {
				for _, k := range keys {
					if k == nil {
						return nil, errInvalidAVOrderSource
					}
				}
			}
			return
		},
		// The shared calculator requires this port for writers, but a preview must
		// never invoke it. No owner write capability is requested or captured.
		reverse: func(*av.AttributeView, *av.Key, *av.Value) error { return errors.New("av_preview_write_forbidden") },
		now: func() time.Time {
			if pending = check(); pending != nil {
				return time.Time{}
			}
			t := e.now()
			pending = e.ctx.Err()
			return t
		}, newID: func() string {
			if pending = check(); pending != nil {
				return ""
			}
			id := e.newID()
			pending = e.ctx.Err()
			if pending == nil && (id == "" || ids[id]) {
				pending = errors.New("invalid_av_preview_generated_id")
			}
			ids[id] = true
			return id
		},
	}
	values, err := attributeViewDefaultValues(a, view, group, previousID, addingID, true, d)
	if err != nil {
		return nil, err
	}
	if err = check(); err != nil {
		return nil, err
	}
	for _, value := range values {
		if value == nil {
			return nil, errInvalidAVOrderSource
		}
		if value.Type == av.KeyTypeBlock {
			if value.Block == nil {
				return nil, errInvalidAVOrderSource
			}
			value.Block.Content = "" // Original anti-flicker rule.
		}
	}
	if err = check(); err != nil {
		return nil, err
	}
	return values, nil
}

func validateAttributeViewPreviewLayout(v *av.View) error {
	if v == nil || !validAttributeViewManagementLayout(v.LayoutType) {
		return errInvalidAVOrderSource
	}
	switch v.LayoutType {
	case av.LayoutTypeTable:
		if v.Table == nil || v.Table.BaseLayout == nil {
			return errInvalidAVOrderSource
		}
	case av.LayoutTypeGallery:
		if v.Gallery == nil || v.Gallery.BaseLayout == nil {
			return errInvalidAVOrderSource
		}
	case av.LayoutTypeKanban:
		if v.Kanban == nil || v.Kanban.BaseLayout == nil {
			return errInvalidAVOrderSource
		}
	}
	return nil
}
