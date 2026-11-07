// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/indexrepair"
)

func (l *Lease) NativeInvalidReferences(ctx context.Context, page, size int) (out docquery.InvalidReferenceResult, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if !r.blockTreeReady {
			if e := r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		boxes, e := r.configurationBusiness.Notebooks(ctx)
		if e != nil {
			return e
		}
		ids := []string{}
		for _, box := range boxes {
			ids = append(ids, box.ID)
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		bt, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		source := indexrepair.Source{Data: w.data}
		if r.configuration.Search == nil {
			return ErrDocument
		}
		out, e = docquery.InvalidReferences(ctx, page, size, docquery.InvalidReferenceServices{
			Walk: func(c context.Context, visit func(*parse.Tree) error) error {
				return docquery.WalkDocumentTrees(c, ids, source.ReadDir, source.Read, parser, visit)
			},
			Blocks: (indexdb.RepairQueries{DB: db}).Blocks,
			Locations: func(c context.Context, ids []string) ([]indexdb.BlockTreeRecord, error) {
				return (indexdb.BlockTrees{DB: bt}).Query(c, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
			},
			CaseSensitive: r.configuration.Search.CaseSensitive,
		})
		return e
	})
	if err != nil {
		return docquery.InvalidReferenceResult{}, err
	}
	return
}
