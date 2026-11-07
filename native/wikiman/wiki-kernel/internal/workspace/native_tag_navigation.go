package workspace

import (
	"context"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/tagquery"
)

// One admission owns both the span query and block projection. This is the
// retained tag grouping helper, not a replacement for UI full-text navigation.
func (l *Lease) NativeTagBlocks(ctx context.Context, keyword string) (ret map[string][]*blockview.Block, err error) {
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r := l.workspace.resources
		if r.configuration.Search == nil {
			return indexdb.ErrProjection
		}
		db, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		ret, e = tagquery.Navigate(ctx, keyword, r.configuration.Search.Limit, r.configuration.Search.CaseSensitive, tagquery.NavigationServices{
			Spans: func(ctx context.Context, keyword string, limit int) ([]indexdb.SpanRecord, error) {
				return indexdb.TagSpans(ctx, db, "keyword", keyword, limit)
			},
			Blocks: func(ctx context.Context, ids []string) (rows []*indexdb.BlockRecord, err error) {
				for _, id := range ids {
					row, found, e := r.nativeBlock(ctx, id)
					if e != nil {
						return nil, e
					}
					if found {
						rows = append(rows, &row)
					}
				}
				return rows, ctx.Err()
			},
		})
		return e
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
