// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"maps"
	"wiki-kernel/internal/assetquery"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
)

func runAssetRead[T any](ctx context.Context, fn func(context.Context, sql.LegacyAssetQueue) (T, error)) (out T, err error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	q := sql.CaptureLegacyAssetQueue()
	if q.DB == nil {
		return out, indexdb.ErrNoDatabase
	}
	err = o.run(ctx, func(op, life context.Context) error {
		if q.DB != sql.CaptureLegacyAssetQueue().DB {
			return sql.ErrAssetQueueOwner
		}
		var e error
		out, e = fn(op, q)
		if e != nil {
			return e
		}
		if q.DB != sql.CaptureLegacyAssetQueue().DB {
			return sql.ErrAssetQueueOwner
		}
		return nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return
}
func FullTextSearchAssetContentContext(ctx context.Context, query string, types map[string]bool, method, order, page, size int) ([]*AssetContent, int, int, error) {
	q := assetquery.Request{Query: query, Types: maps.Clone(types), Method: method, OrderBy: order, Page: page, PageSize: size}
	v, e := runAssetRead(ctx, func(c context.Context, db sql.LegacyAssetQueue) (assetquery.Result, error) {
		return assetquery.Search(c, db.DB, q)
	})
	return v.Contents, v.Matched, v.PageCount, e
}
func GetAssetContentContext(ctx context.Context, id, query string, method int) (*AssetContent, error) {
	return runAssetRead(ctx, func(c context.Context, q sql.LegacyAssetQueue) (*AssetContent, error) {
		return assetquery.Detail(c, q.DB, id, query, method)
	})
}
