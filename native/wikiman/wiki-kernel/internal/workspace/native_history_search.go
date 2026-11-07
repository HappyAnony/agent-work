// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"time"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) ReplaceNativeHistoryProjection(ctx context.Context, directory string, rows []indexdb.HistoryEntry) error {
	return l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, err := bundle.Database(indexdb.History)
		if err != nil {
			return err
		}
		var pending indexdb.HistoryQueue
		if err = pending.ReplaceDirectory(directory, rows); err != nil {
			return err
		}
		return pending.Flush(ctx, db, nil)
	})
}

func (l *Lease) NativeHistorySearch(ctx context.Context, q indexdb.HistorySearchRequest) (out indexdb.HistorySearchResult, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r := l.workspace.resources
		if r.configuration.Editor == nil {
			return indexdb.ErrProjection
		}
		q.RetentionDays = r.configuration.Editor.HistoryRetentionDays
		q.Now = time.Now()
		db, e := b.Database(indexdb.History)
		if e != nil {
			return e
		}
		out, e = indexdb.SearchHistory(ctx, db, q)
		return e
	})
	if err != nil {
		return indexdb.HistorySearchResult{}, err
	}
	return
}
func (l *Lease) NativeHistorySearchItems(ctx context.Context, q indexdb.HistorySearchRequest, created string) (out []indexdb.HistoryEntry, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r := l.workspace.resources
		if r.configuration.Editor == nil {
			return indexdb.ErrProjection
		}
		q.RetentionDays = r.configuration.Editor.HistoryRetentionDays
		q.Now = time.Now()
		db, e := b.Database(indexdb.History)
		if e != nil {
			return e
		}
		out, e = indexdb.SearchHistoryItems(ctx, db, q, created)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
