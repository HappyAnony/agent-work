// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"slices"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
)

func GetBlockDOMsContext(ctx context.Context, ids []string, embed bool) (map[string]string, error) {
	ids = slices.Clone(ids)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (map[string]string, error) {
		return blockview.DOMs(c, ids, embed, blockview.DOMServices{Parser: o.parser, Trees: o.trees, Load: o.loadOptional, Select: func(c context.Context, stmt string) ([]indexdb.BlockRecord, error) {
			if o.settings.Search == nil {
				return nil, errors.New("block_search_settings_missing")
			}
			if e := o.r.guard(c); e != nil {
				return nil, e
			}
			return indexdb.SelectReadBlockRows(c, o.r.main.DB, stmt, 1, o.settings.Search.Limit)
		}})
	})
}
func GetBlockDOMContext(ctx context.Context, id string, embed bool) (string, error) {
	v, e := GetBlockDOMsContext(ctx, []string{id}, embed)
	return v[id], e
}
