// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"os"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
)

// AV post-import business only. This does not turn the legacy archive/file-copy
// controller into a native importer or replace R04's authoritative source commit.
type attributeViewImportEnvironment struct {
	ownerGuard         func() error
	complete           func() error
	strictDependencies bool
	ctx                context.Context
	legacy             bool
	read               func(string) (*av.AttributeView, error)
	bindings           *attributeViewDuplicateBindingsEnvironment
	registry           func([]*ast.Node) error
	relation           func(string, string) error
}

func (e *attributeViewImportEnvironment) validate(finalize bool) error {
	if e == nil || e.ctx == nil || e.read == nil || e.bindings == nil {
		return errors.New("av_import_dependencies_unbound")
	}
	b := e.bindings
	if b.exists == nil || b.node == nil || b.name == nil || b.untitled == nil || b.attrs == nil || b.cache == nil || b.notify == nil || b.tree == nil {
		return errors.New("av_import_binding_dependencies_unbound")
	}
	if finalize && (e.registry == nil || e.relation == nil) {
		return errors.New("av_import_finalize_dependencies_unbound")
	}
	return e.ctx.Err()
}

func legacyAttributeViewImportEnvironment(contexts ...context.Context) *attributeViewImportEnvironment {
	ctx := legacyAVContext(contexts)
	b := newLegacyAVBinding(ctx)
	return importWithBinding(nil, b, ctx)
}
func importWithBinding(tx *Transaction, b *legacyAVBinding, ctx context.Context) *attributeViewImportEnvironment {
	return &attributeViewImportEnvironment{ctx: ctx, legacy: true, ownerGuard: b.liveSourceGuard, complete: b.complete, strictDependencies: true, read: b.readCandidate,
		// Reuse the adapter, without running any duplicate operation or generating IDs.
		bindings: duplicateBindings(tx, b),
		registry: b.registerMirrors,
		relation: b.upsertRelation,
	}
}

func (tx *Transaction) attributeViewImportEnvironment() *attributeViewImportEnvironment {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.avImport == nil {
			return nil
		}
		e := *tx.environment.avImport
		e.ctx, e.legacy = tx.environment.ctx, false
		return &e
	}
	return importWithBinding(tx, legacyAVBindingForTransaction(tx), transactionAVContext(tx))
}

func (e *attributeViewImportEnvironment) guard(fn func() error) error {
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
	if e.ownerGuard != nil {
		return e.ownerGuard()
	}
	return e.ctx.Err()
}

func (tx *Transaction) updateBoundBlockAvsAttribute(ids []string) error {
	return updateBoundBlockAvsAttributeWithEnvironment(ids, tx.attributeViewImportEnvironment())
}

func updateBoundBlockAvsAttributeWithEnvironment(ids []string, e *attributeViewImportEnvironment) error {
	if err := e.validate(false); err != nil {
		return err
	}
	if err := refreshImportedAttributeViewBindings(ids, e); err != nil {
		return err
	}
	return e.finish()
}
func (e *attributeViewImportEnvironment) finish() error {
	if err := e.guard(func() error { return nil }); err != nil {
		return err
	}
	if e.complete != nil {
		return e.complete()
	}
	return nil
}
func refreshImportedAttributeViewBindings(ids []string, e *attributeViewImportEnvironment) error {
	views, err := readImportedAttributeViews(ids, e)
	if err != nil {
		return err
	}
	return refreshAttributeViewBoundAttributes(views, e.ctx, e.legacy, e.bindings, e.guard)
}

func readImportedAttributeViews(ids []string, e *attributeViewImportEnvironment) ([]*av.AttributeView, error) {
	for _, id := range ids {
		if id == "" {
			return nil, errors.New("invalid_av_import_id")
		}
	}
	var views []*av.AttributeView
	for _, id := range ids {
		var a *av.AttributeView
		if err := e.guard(func() (err error) { a, err = e.read(id); return }); err != nil {
			if e.legacy && (!e.strictDependencies || os.IsNotExist(err)) && e.ctx.Err() == nil {
				continue
			}
			return nil, err
		}
		if a == nil && e.legacy && !e.strictDependencies {
			continue
		}
		if err := validateCellSource(a); err != nil {
			return nil, err
		}
		if a.ID != id || a.GetBlockKeyValues() == nil {
			return nil, errors.New("invalid_av_import_source")
		}
		views = append(views, a)
	}
	return views, nil
}

func (tx *Transaction) finishImportedAttributeViews(ids []string, trees []*parse.Tree) error {
	return finishImportedAttributeViewsWithEnvironment(ids, trees, tx.attributeViewImportEnvironment())
}

// Preserve ImportSY's ordering: imported mirror registration, batch binding
// refresh, then the original one-destination-per-source relation projection.
func finishImportedAttributeViewsWithEnvironment(ids []string, trees []*parse.Tree, e *attributeViewImportEnvironment) error {
	if err := e.validate(true); err != nil {
		return err
	}
	for _, id := range ids {
		if id == "" {
			return errors.New("invalid_av_import_id")
		}
	}
	for _, tree := range trees {
		// Archive trees have remapped IDs but are not mounted into a notebook yet.
		if tree == nil || tree.Root == nil || tree.ID == "" {
			return errors.New("invalid_av_import_tree")
		}
	}
	for _, tree := range trees {
		if err := e.guard(func() error { return e.registry(tree.Root.ChildrenByType(ast.NodeAttributeView)) }); err != nil {
			return err
		}
	}
	if err := refreshImportedAttributeViewBindings(ids, e); err != nil {
		return err
	}
	// Keep the second read after bindings as in the original import call chain.
	views, err := readImportedAttributeViews(ids, e)
	if err != nil {
		return err
	}
	links := map[string]string{}
	var sources []string
	for _, a := range views {
		dest, found := "", false
		for _, kv := range a.KeyValues {
			if kv.Key.Type == av.KeyTypeRelation && kv.Key.Relation != nil {
				dest, found = kv.Key.Relation.AvID, true
			}
		}
		if found {
			if _, seen := links[a.ID]; !seen {
				sources = append(sources, a.ID)
			}
			links[a.ID] = dest
		}
	}
	for _, src := range sources {
		if err := e.guard(func() error { return e.relation(src, links[src]) }); err != nil {
			return err
		}
	}
	return e.finish()
}
