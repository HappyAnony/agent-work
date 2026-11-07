// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/indexdb"
)

var ErrReferenceDependency = errors.New("reference_refresh_unbound")
var ErrReferenceTree = errors.New("reference_refresh_invalid_tree")

type ReferenceServices struct {
	References func(context.Context, string) ([]indexdb.RefRecord, error)
	Load       func(context.Context, string) (*parse.Tree, error)
	Lookup     func(context.Context, string) (*indexdb.BlockTreeRecord, error)
	Index      func(context.Context, *parse.Tree) error
	Flush      func(context.Context) error
}

// RefreshReferences preserves original referenced-root grouping and count IDs.
// All source reads finish before enqueue. Count IDs are returned only after
// accepted derived writes really flush; scheduling/UI remain owner callbacks.
func RefreshReferences(ctx context.Context, id string, s ReferenceServices) ([]string, error) {
	if s.References == nil || s.Load == nil || s.Lookup == nil || s.Index == nil || s.Flush == nil {
		return nil, ErrReferenceDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	refs, e := s.References(ctx, id)
	if e != nil {
		return nil, e
	}
	var roots, counts []string
	seen := map[string]bool{}
	counted := map[string]bool{}
	add := func(id string) {
		if id != "" && !counted[id] {
			counted[id] = true
			counts = append(counts, id)
		}
	}
	for _, ref := range refs {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		add(ref.DefBlockID)
		if !seen[ref.RootID] {
			seen[ref.RootID] = true
			roots = append(roots, ref.RootID)
		}
	}
	var trees []*parse.Tree
	for _, root := range roots {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		t, e := s.Load(ctx, root)
		if e != nil {
			return nil, e
		}
		if t == nil || t.Root == nil || t.ID != root || t.Root.ID != root {
			return nil, ErrReferenceTree
		}
		trees = append(trees, t)
		add(t.ID)
	}
	bt, e := s.Lookup(ctx, id)
	if e != nil {
		return nil, e
	}
	if bt != nil {
		add(id)
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	for _, tree := range trees {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if e = s.Index(ctx, tree); e != nil {
			return nil, e
		}
	}
	if e = s.Flush(ctx); e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return counts, nil
}

type ReferenceCount struct {
	RootID, BlockID  string
	DefinitionIDs    []string
	Count, RootCount int
}
type CountServices struct {
	Lookup     func(context.Context, string) (*indexdb.BlockTreeRecord, error)
	References func(context.Context, string, bool) ([]string, error)
	Children   func(context.Context, string) ([]string, error)
}

func ReadReferenceCount(ctx context.Context, id string, s CountServices) (*ReferenceCount, error) {
	if s.Lookup == nil || s.References == nil || s.Children == nil {
		return nil, ErrReferenceDependency
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	bt, e := s.Lookup(ctx, id)
	if e != nil {
		return nil, e
	}
	if bt == nil {
		return nil, ctx.Err()
	}
	if bt.ID != id || bt.RootID == "" {
		return nil, ErrReferenceTree
	}
	doc := bt.ID == bt.RootID
	refs, e := s.References(ctx, bt.ID, doc)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	roots := refs
	if !doc {
		roots, e = s.References(ctx, bt.RootID, true)
		if e != nil {
			return nil, e
		}
	}
	defs := []string{id}
	if doc {
		defs, e = s.Children(ctx, bt.ID)
		if e != nil {
			return nil, e
		}
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return &ReferenceCount{RootID: bt.RootID, BlockID: id, DefinitionIDs: append([]string(nil), defs...), Count: len(refs), RootCount: len(roots)}, nil
}
