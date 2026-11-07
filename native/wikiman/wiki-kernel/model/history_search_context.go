// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"path/filepath"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
)

func runHistoryRead[T any](ctx context.Context, fn func(context.Context, *legacyReindexOwner, sql.LegacyHistoryQueue, conf.AppData) (T, error)) (out T, err error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return out, e
	}
	q := sql.CaptureLegacyHistoryQueue()
	if q.DB == nil {
		return out, indexdb.ErrNoDatabase
	}
	var settings conf.AppData
	if e = o.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error { settings = v; return o.guard(ctx) }); e != nil {
		return out, e
	}
	err = o.run(ctx, func(op, life context.Context) error {
		if q.DB != sql.CaptureLegacyHistoryQueue().DB {
			return conf.ErrBusinessUnbound
		}
		var e error
		out, e = fn(op, o, q, settings)
		if e != nil {
			return e
		}
		if q.DB != sql.CaptureLegacyHistoryQueue().DB {
			return conf.ErrBusinessUnbound
		}
		return nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return
}

func FullTextSearchHistoryContext(ctx context.Context, query, box, op string, typ, page int) ([]string, int, int, error) {
	v, e := runHistoryRead(ctx, func(c context.Context, o *legacyReindexOwner, db sql.LegacyHistoryQueue, s conf.AppData) (indexdb.HistorySearchResult, error) {
		if s.Editor == nil {
			return indexdb.HistorySearchResult{}, conf.ErrBusinessUnbound
		}
		return indexdb.SearchHistory(c, db.DB, indexdb.HistorySearchRequest{Query: query, Box: box, Operation: op, Type: typ, Page: page, RetentionDays: s.Editor.HistoryRetentionDays, Now: time.Now()})
	})
	return v.Created, v.Pages, v.Total, e
}
func FullTextSearchHistoryItemsContext(ctx context.Context, created, query, box, op string, typ int) ([]*HistoryItem, error) {
	return runHistoryRead(ctx, func(c context.Context, o *legacyReindexOwner, db sql.LegacyHistoryQueue, s conf.AppData) ([]*HistoryItem, error) {
		if s.Editor == nil {
			return nil, conf.ErrBusinessUnbound
		}
		rows, e := indexdb.SearchHistoryItems(c, db.DB, indexdb.HistorySearchRequest{Query: query, Box: box, Operation: op, Type: typ, RetentionDays: s.Editor.HistoryRetentionDays, Now: time.Now()}, created)
		if e != nil {
			return nil, e
		}
		out := []*HistoryItem{}
		for _, row := range rows {
			item := &HistoryItem{Title: row.Title, Path: filepath.Join(o.history, row.Path), Op: row.Op}
			if row.Type == HistoryTypeAsset {
				item.Path = filepath.ToSlash(strings.TrimPrefix(item.Path, o.workspace))
			} else {
				parts := strings.Split(row.Path, "/")
				if len(parts) >= 2 {
					item.Notebook = parts[1]
				}
			}
			out = append(out, item)
		}
		return out, c.Err()
	})
}
