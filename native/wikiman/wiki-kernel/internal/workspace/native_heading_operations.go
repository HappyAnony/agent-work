// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
)

type NativeHeadingOperationsPlan struct {
	Plan         doclife.HeadingOperationsPlan
	SourceHashes map[string][32]byte
}

func (l *Lease) PlanNativeHeadingOperations(ctx context.Context, q doclife.HeadingOperationsRequest) (out NativeHeadingOperationsPlan, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		s, hashes, guard, e := r.documentLifecycleServices(ctx, w, b, "")
		if e != nil {
			return e
		}
		loc, ok, e := s.Lookup(ctx, q.ID)
		if e != nil {
			return e
		}
		if !ok {
			return ErrDocument
		}
		tree, e := s.Load(ctx, loc.Box, loc.Path)
		if e != nil {
			return e
		}
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		plan, e := doclife.PrepareHeadingOperations(ctx, tree, q, doclife.HeadingOperationsServices{Parser: parser, NewID: ast.NewNodeID, Counts: (indexdb.References{DB: db}).DefinitionCounts})
		if e != nil {
			return e
		}
		if e = guard.Confirm(ctx); e != nil {
			return e
		}
		out = NativeHeadingOperationsPlan{Plan: plan, SourceHashes: hashes}
		return nil
	})
	if err != nil {
		return NativeHeadingOperationsPlan{}, err
	}
	return
}

func (l *Lease) PlanNativeAppendHeadingChildren(ctx context.Context, id, dom string) (out NativeFormatPlan, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		s, hashes, guard, e := r.documentLifecycleServices(ctx, w, b, "")
		if e != nil {
			return e
		}
		loc, ok, e := s.Lookup(ctx, id)
		if e != nil {
			return e
		}
		if !ok {
			return ErrDocument
		}
		tree, e := s.Load(ctx, loc.Box, loc.Path)
		if e != nil {
			return e
		}
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		if _, e = doclife.AppendHeadingChildren(ctx, tree, id, dom, parser); e != nil {
			return e
		}
		if e = guard.Confirm(ctx); e != nil {
			return e
		}
		out = NativeFormatPlan{Tree: tree, SourceHashes: hashes}
		return nil
	})
	if err != nil {
		return NativeFormatPlan{}, err
	}
	return
}
