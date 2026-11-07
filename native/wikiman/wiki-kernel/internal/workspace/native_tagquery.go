package workspace

import (
	"context"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) SearchNativeTags(ctx context.Context, keyword string) (ret []string, err error) {
	// Match original SearchTags' own main-index flush, not a global worker.
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		return nil, err
	}
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r := l.workspace.resources
		if r.configuration.Search == nil {
			return indexdb.ErrProjection
		}
		db, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		ret, e = indexdb.SearchTagLabels(ctx, db, keyword, r.configuration.Search.Limit, r.configuration.Search.CaseSensitive)
		return e
	})
	return
}
func (l *Lease) NativeTagSpans(ctx context.Context, kind, value string, limit int) (ret []indexdb.SpanRecord, err error) {
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		ret, e = indexdb.TagSpans(ctx, db, kind, value, limit)
		return e
	})
	return
}
