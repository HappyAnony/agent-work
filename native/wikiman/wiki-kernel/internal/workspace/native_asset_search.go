// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"maps"
	"wiki-kernel/internal/assetquery"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) SearchNativeAssetContents(ctx context.Context, q assetquery.Request) (out assetquery.Result, err error) {
	q.Types = maps.Clone(q.Types)
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.Assets)
		if e != nil {
			return e
		}
		out, e = assetquery.Search(ctx, db, q)
		return e
	})
	if err != nil {
		return assetquery.Result{}, err
	}
	return
}
func (l *Lease) NativeAssetContent(ctx context.Context, id, query string, method int) (out *assetquery.Content, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		db, e := b.Database(indexdb.Assets)
		if e != nil {
			return e
		}
		out, e = assetquery.Detail(ctx, db, id, query, method)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
