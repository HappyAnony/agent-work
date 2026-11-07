// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
)

func legacySearchViewServices() blockview.Services {
	return blockview.Services{
		Blocks: func(_ context.Context, ids []string) ([]*indexdb.BlockRecord, error) {
			var ret []*indexdb.BlockRecord
			for _, b := range sql.GetBlocks(ids) {
				if b != nil {
					r := indexdb.BlockRecord(*b)
					ret = append(ret, &r)
				}
			}
			return ret, nil
		},
		Tree: func(_ context.Context, id string) (*parse.Tree, error) {
			return loadTreeByBlockTree(treenode.GetBlockTree(id))
		},
		ReferenceCounts: func(_ context.Context, ids []string) (map[string]int, error) { return sql.QueryRefCount(ids), nil },
	}
}
