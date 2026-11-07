// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
)

type NativeHeadingMovePlan struct {
	Tree         *parse.Tree
	Changed      bool
	SourceHashes map[string][32]byte
}

func (l *Lease) PlanNativeHeadingMove(ctx context.Context, q doclife.HeadingMoveRequest) (out NativeHeadingMovePlan, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		s, hashes, guard, e := l.workspace.resources.documentLifecycleServices(ctx, l.workspace, b, "")
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
		changed, e := doclife.MoveHeading(ctx, tree, q, func(c context.Context, _ *parse.Tree) error { return c.Err() })
		if e != nil {
			return e
		}
		if e = guard.Confirm(ctx); e != nil {
			return e
		}
		out = NativeHeadingMovePlan{Tree: tree, Changed: changed, SourceHashes: hashes}
		return nil
	})
	if err != nil {
		return NativeHeadingMovePlan{}, err
	}
	return
}
