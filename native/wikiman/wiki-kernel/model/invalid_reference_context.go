// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func ListInvalidBlockRefsContext(ctx context.Context, page, size int) ([]*Block, int, int, int, error) {
	v, e := runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (out docquery.InvalidReferenceResult, err error) {
		if o.settings.Search == nil {
			return out, errors.New("invalid_reference_search_settings")
		}
		boxes, e := o.base.owner.business.Notebooks(c)
		if e != nil {
			return out, e
		}
		ids := []string{}
		for _, box := range boxes {
			ids = append(ids, box.ID)
		}
		ioctx, closeIO, e := o.r.openRepairIOChecked(c, false)
		if e != nil {
			return out, e
		}
		defer func() {
			err = errors.Join(err, closeIO())
			if err != nil {
				out = docquery.InvalidReferenceResult{}
			}
		}()
		return docquery.InvalidReferences(c, page, size, docquery.InvalidReferenceServices{
			Walk: func(c context.Context, visit func(*parse.Tree) error) error {
				return docquery.WalkDocumentTrees(c, ids, ioctx.Source.ReadDir, ioctx.Source.Read, o.parser, visit)
			},
			Blocks: o.r.main.Blocks,
			Locations: func(c context.Context, ids []string) ([]indexdb.BlockTreeRecord, error) {
				return o.r.registry.Query(c, indexdb.BlockTreeFilter{Kind: "ids", IDs: ids})
			},
			CaseSensitive: o.settings.Search.CaseSensitive,
		})
	})
	return v.Blocks, v.MatchedBlocks, v.MatchedRoots, v.PageCount, e
}
