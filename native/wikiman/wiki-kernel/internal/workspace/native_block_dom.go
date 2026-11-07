// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"github.com/88250/lute/parse"
	"slices"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeBlockDOMs(ctx context.Context, ids []string, embed bool) (out map[string]string, err error) {
	ids = slices.Clone(ids)
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		out, e = blockview.DOMs(ctx, ids, embed, blockview.DOMServices{Parser: parser, Trees: func(c context.Context, ids []string) (map[string]*parse.Tree, error) {
			return r.documentInfoTrees(c, w, ids)
		}, Load: func(c context.Context, id string) (*parse.Tree, error) {
			trees, e := r.documentInfoTrees(c, w, []string{id})
			return trees[id], e
		}, Select: func(c context.Context, stmt string) ([]indexdb.BlockRecord, error) {
			if r.configuration.Search == nil {
				return nil, indexdb.ErrProjection
			}
			return indexdb.SelectReadBlockRows(c, db, stmt, 1, r.configuration.Search.Limit)
		}})
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
