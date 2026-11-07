// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
)

func SearchRefBlockContext(ctx context.Context, id, rootID, keyword string, beforeLen int, square, database bool) ([]*Block, bool, error) {
	v, e := runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (blockview.ReferenceSearchResult, error) {
		return blockview.SearchReferences(c, blockview.ReferenceSearchRequest{ID: id, RootID: rootID, Keyword: keyword, BeforeLen: beforeLen, SquareBrackets: square, Database: database}, blockview.ReferenceSearchServices{
			DB: o.r.main.DB, Search: o.settings.Search, Editor: o.settings.Editor, Parser: o.parser, Content: o.content(), Load: o.loadOptional,
			Ignore: func(c context.Context) ([]string, error) { return o.ignore(c, true) }, Notebooks: o.base.owner.business.Notebooks, Location: o.r.referenceLookup,
			HasPath: func(c context.Context, box, p string) (bool, error) {
				if e := o.r.guard(c); e != nil {
					return false, e
				}
				rows, e := o.r.registry.Query(c, indexdb.BlockTreeFilter{Kind: "root-hpath", BoxID: box, Value: p})
				return len(rows) > 0, e
			},
		})
	})
	return v.Blocks, v.NewDoc, e
}
