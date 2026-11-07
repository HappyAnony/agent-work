// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"

	"wiki-kernel/av"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
)

// Image queries reuse the original table projection/render/filter/sort business.
// Native render dependencies own any repair candidates; this is not R04 commit.
type attributeViewImagesEnvironment struct {
	ctx     context.Context
	read    func(string) (*av.AttributeView, error)
	clone   func(*av.AttributeView) (*av.AttributeView, error)
	further func(*av.AttributeView, map[string]*av.AttributeView) (map[string]av.Collection, error)
	render  func(*av.AttributeView, *av.View, string) (*av.Table, error)
	filter  func(*av.Table, *av.AttributeView, map[string]av.Collection, map[string]*av.AttributeView) error
	sort    func(*av.Table, *av.AttributeView) error
}

func legacyAttributeViewImagesEnvironment(contexts ...context.Context) *attributeViewImagesEnvironment {
	ctx := legacyAVContext(contexts)
	return imagesWithBinding(readonlyAVBinding(nil, ctx), ctx)
}
func imagesWithBinding(b *legacyAVBinding, ctx context.Context) *attributeViewImagesEnvironment {
	return &attributeViewImagesEnvironment{ctx: ctx, read: b.readCandidate, clone: b.clone,
		further: func(a *av.AttributeView, c map[string]*av.AttributeView) (map[string]av.Collection, error) {
			return avrender.FurtherCollections(ctx, a, c, avrender.FurtherEnvironment{Read: b.scope.read, Render: b.scope.render, Missing: os.IsNotExist})
		},
		render: func(a *av.AttributeView, v *av.View, q string) (*av.Table, error) {
			if err := b.adopt(a); err != nil {
				return nil, err
			}
			out, err := b.scope.renderAs(a, v, q, av.LayoutTypeTable)
			if err != nil {
				return nil, err
			}
			table, ok := out.(*av.Table)
			if !ok {
				return nil, avrender.ErrShape
			}
			return table, nil
		},
		filter: func(t *av.Table, a *av.AttributeView, f map[string]av.Collection, c map[string]*av.AttributeView) error {
			if err := b.guard(); err != nil {
				return err
			}
			return avmodel.FilterChecked(t, a, f, c)
		},
		sort: func(t *av.Table, a *av.AttributeView) error {
			if err := b.guard(); err != nil {
				return err
			}
			av.Sort(t, a)
			return b.guard()
		}}
}
func (tx *Transaction) attributeViewImagesEnvironment() *attributeViewImagesEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avImages == nil {
			return nil
		}
		e := *tx.environment.avImages
		e.ctx = tx.environment.ctx
		return &e
	}
	ctx := transactionAVContext(tx)
	return imagesWithBinding(readonlyAVBinding(tx, ctx), ctx)
}

func (e *attributeViewImagesEnvironment) validate() error {
	if e == nil || e.ctx == nil || e.read == nil || e.clone == nil || e.further == nil || e.render == nil || e.filter == nil || e.sort == nil {
		return errors.New("av_images_dependencies_unbound")
	}
	return e.ctx.Err()
}
func (e *attributeViewImagesEnvironment) guard(fn func() error) error {
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	return e.ctx.Err()
}

// Deliberately keep ID-only gallery/kanban columns: the original exporter does
// not transfer card-field Hidden flags. Table column visibility stays intact.
func projectAttributeViewTable(view *av.View) { avrender.ProjectTable(view) }

func (tx *Transaction) currentAttributeViewImages(id, viewID, query string) ([]string, error) {
	return currentAttributeViewImagesWithEnvironment(id, viewID, query, tx.attributeViewImagesEnvironment())
}

func currentAttributeViewImagesWithEnvironment(id, viewID, query string, e *attributeViewImagesEnvironment) ([]string, error) {
	if err := e.validate(); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, errors.New("invalid_av_images_id")
	}
	var source, a *av.AttributeView
	if err := e.guard(func() (err error) { source, err = e.read(id); return }); err != nil {
		return nil, err
	}
	if err := validateImageAttributeView(source, id); err != nil {
		return nil, err
	}
	if err := e.guard(func() (err error) { a, err = e.clone(source); return }); err != nil {
		return nil, err
	}
	if a == source {
		return nil, errors.New("invalid_av_images_clone")
	}
	if err := validateImageAttributeView(a, id); err != nil {
		return nil, err
	}
	var view *av.View
	if viewID != "" {
		view, _ = a.GetCurrentView(viewID)
	} else {
		view = a.GetView(a.ViewID)
	}
	if err := validateAttributeViewPreviewLayout(view); err != nil {
		return nil, err
	}
	cache := map[string]*av.AttributeView{}
	var further map[string]av.Collection
	if err := e.guard(func() (err error) { further, err = e.further(a, cache); return }); err != nil {
		return nil, err
	}
	projectAttributeViewTable(view)
	var table *av.Table
	if err := e.guard(func() (err error) { table, err = e.render(a, view, query); return }); err != nil {
		return nil, err
	}
	if err := validateImageTable(table); err != nil {
		return nil, err
	}
	if err := e.guard(func() error { return e.filter(table, a, further, cache) }); err != nil {
		return nil, err
	}
	if err := e.guard(func() error { return e.sort(table, a) }); err != nil {
		return nil, err
	}
	if err := validateImageTable(table); err != nil {
		return nil, err
	}
	return avrender.Images(e.ctx, table)
}

func validateImageAttributeView(a *av.AttributeView, id string) error {
	if a == nil || a.ID != id {
		return errors.New("invalid_av_images_source")
	}
	if err := validateUpdatedAttributeView(a); err != nil {
		return err
	}
	for _, kv := range a.KeyValues {
		for _, v := range kv.Values {
			for _, asset := range v.MAsset {
				if asset == nil {
					return errors.New("invalid_av_images_asset")
				}
			}
		}
	}
	for _, view := range a.Views {
		for _, filter := range view.Filters {
			if filter == nil {
				return errors.New("invalid_av_images_filter")
			}
		}
		for _, sort := range view.Sorts {
			if sort == nil {
				return errors.New("invalid_av_images_sort")
			}
		}
	}
	return validateAttributeViewFieldSource(a)
}
func validateImageTable(t *av.Table) error {
	if t == nil {
		return errors.New("invalid_av_images_table")
	}
	for _, c := range t.Columns {
		if c == nil || c.BaseInstanceField == nil {
			return errors.New("invalid_av_images_column")
		}
	}
	for _, r := range t.Rows {
		if r == nil {
			return errors.New("invalid_av_images_row")
		}
		if len(r.Cells) != len(t.Columns) {
			return errors.New("invalid_av_images_cells")
		}
		for _, c := range r.Cells {
			if c == nil {
				return errors.New("invalid_av_images_cell")
			}
		}
	}
	return nil
}
