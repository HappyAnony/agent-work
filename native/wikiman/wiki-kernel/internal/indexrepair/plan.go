// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
// Package indexrepair preserves the original index consistency rules with
// explicit databases. Plans convey no source or derived-index write authority.
package indexrepair

import (
	"context"
	"slices"
	"time"
	"wiki-kernel/internal/indexdb"
)

type Plan struct {
	DuplicateRoots          []string
	ReindexRoots            []string
	RemoveRoots             []string
	DuplicateReferenceRoots []string
}

func DuplicateRoots(ctx context.Context, q indexdb.RepairQueries, caseSensitive bool) ([]string, error) {
	ids, e := q.DuplicatedRoots(ctx, "blocks")
	if e != nil {
		return nil, e
	}
	if len(ids) == 0 {
		ids, e = q.DuplicatedRoots(ctx, "blocks_fts")
		if e != nil {
			return nil, e
		}
		if len(ids) == 0 && !caseSensitive {
			ids, e = q.DuplicatedRoots(ctx, "blocks_fts_case_insensitive")
			if e != nil {
				return nil, e
			}
		}
	}
	blocks, e := q.Blocks(ctx, ids)
	if e != nil {
		return nil, e
	}
	var ret []string
	for _, b := range blocks {
		if b != nil {
			ret = append(ret, b.ID)
		}
	}
	slices.Sort(ret)
	return slices.Compact(ret), ctx.Err()
}

// Reconcile uses the original ten-minute tolerance and pre-updated-field
// migration rule. Reads complete before the caller may execute any correction.
func Reconcile(ctx context.Context, q indexdb.RepairQueries, trees indexdb.BlockTrees) (reindex, remove []string, err error) {
	roots, e := trees.RootUpdated(ctx)
	if e != nil {
		return nil, nil, e
	}
	database, e := q.RootUpdated(ctx)
	if e != nil {
		return nil, nil, e
	}
	for id, updated := range roots {
		if e = ctx.Err(); e != nil {
			return nil, nil, e
		}
		prior := database[id]
		if prior == "" || updated == "" {
			reindex = append(reindex, id)
			continue
		}
		bt, _ := time.Parse("20060102150405", updated)
		db, _ := time.Parse("20060102150405", prior)
		if db.Before(bt.Add(-10 * time.Minute)) {
			reindex = append(reindex, id)
		}
	}
	var candidates []string
	for id := range database {
		if e = ctx.Err(); e != nil {
			return nil, nil, e
		}
		if _, ok := roots[id]; !ok {
			candidates = append(candidates, id)
		}
	}
	blocks, e := q.Blocks(ctx, candidates)
	if e != nil {
		return nil, nil, e
	}
	for _, b := range blocks {
		if b != nil {
			remove = append(remove, b.RootID)
		}
	}
	slices.Sort(reindex)
	slices.Sort(remove)
	return slices.Compact(reindex), slices.Compact(remove), ctx.Err()
}
func Inspect(ctx context.Context, q indexdb.RepairQueries, trees indexdb.BlockTrees, caseSensitive bool) (out Plan, err error) {
	out.DuplicateRoots, err = DuplicateRoots(ctx, q, caseSensitive)
	if err != nil {
		return Plan{}, err
	}
	out.ReindexRoots, out.RemoveRoots, err = Reconcile(ctx, q, trees)
	if err != nil {
		return Plan{}, err
	}
	out.DuplicateReferenceRoots, err = q.DuplicatedReferenceRoots(ctx)
	if err != nil {
		return Plan{}, err
	}
	slices.Sort(out.DuplicateReferenceRoots)
	if err = ctx.Err(); err != nil {
		return Plan{}, err
	}
	return out, nil
}
