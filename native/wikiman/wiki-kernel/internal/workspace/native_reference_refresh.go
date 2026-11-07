// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/nativeparser"
)

// RefreshNativeReferenceIndex changes only this page's derived reference index.
// Counts are returned after actual flush; no delayed UI task or source write.
func (l *Lease) RefreshNativeReferenceIndex(ctx context.Context, id string) (out []indexrepair.ReferenceCount, err error) {
	pin, err := l.pin(true)
	if err != nil {
		return nil, err
	}
	defer pin.Release()
	if !documentID.MatchString(id) {
		return nil, ErrDocument
	}
	if err = pin.FlushNativeMainQueue(ctx); err != nil {
		return nil, err
	}
	var events []IndexEvent
	err = pin.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := pin.workspace.resources, pin.workspace
		if !r.blockTreeReady {
			if e := r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		registryDB, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		refs := indexdb.References{DB: db}
		registry := indexdb.BlockTrees{DB: registryDB}
		lookup := func(ctx context.Context, id string) (*indexdb.BlockTreeRecord, error) {
			rows, e := registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
			if e != nil {
				return nil, e
			}
			if len(rows) == 0 {
				return nil, nil
			}
			return &rows[0], nil
		}
		ids, e := indexrepair.RefreshReferences(ctx, id, indexrepair.ReferenceServices{
			References: func(ctx context.Context, id string) ([]indexdb.RefRecord, error) {
				return refs.ByDefinition(ctx, id, true)
			}, Lookup: lookup,
			Load: func(ctx context.Context, id string) (*parse.Tree, error) {
				trees, e := r.documentInfoTrees(ctx, w, []string{id})
				if e != nil {
					return nil, e
				}
				return trees[id], nil
			},
			Index: func(ctx context.Context, t *parse.Tree) error {
				settings, e := r.configuredParserOptions(ctx, w)
				if e != nil {
					return e
				}
				rows, annotations, e := indexdb.ProjectReferences(ctx, t, nativeparser.NewConfiguredIndex(settings), r.nativeDefinition)
				if e != nil {
					return e
				}
				return r.contentQueue.Enqueue(indexdb.MainMutation{Action: "update_refs", ID: t.ID, Box: t.Box, Path: t.Path, Refs: rows, Annotations: annotations})
			}, Flush: func(ctx context.Context) error { return pin.flushNativeMainLocked(ctx, b, &events) },
		})
		if e != nil {
			return e
		}
		for _, id := range ids {
			count, e := indexrepair.ReadReferenceCount(ctx, id, indexrepair.CountServices{Lookup: lookup, References: refs.RefIDs, Children: refs.ChildDefinitionIDs})
			if e != nil {
				return e
			}
			if count != nil {
				out = append(out, *count)
			}
		}
		return ctx.Err()
	})
	for _, event := range events {
		err = errors.Join(err, pin.workspace.resources.indexEvents.Publish(event))
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}
