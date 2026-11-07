package workspace

import (
	"context"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/markup"
	"wiki-kernel/internal/tagquery"
)

type NativeTagTree struct {
	Tags     tagquery.Tags `json:"tags"`
	Exceeded bool          `json:"exceeded"`
}

func (l *Lease) BuildNativeTags(ctx context.Context, ignoreLimit bool) (ret NativeTagTree, err error) {
	return l.BuildNativeTagsWithSort(ctx, ignoreLimit, nil)
}

func (l *Lease) BuildNativeTagsWithSort(ctx context.Context, ignoreLimit bool, sortMode *int) (ret NativeTagTree, err error) {
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		return ret, err
	}
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r := l.workspace.resources
		if r.configuration.Tag == nil || r.configuration.FileTree == nil {
			return indexdb.ErrProjection
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		spans, e := indexdb.TagSpans(ctx, db, "path", "", 0)
		if e != nil {
			return e
		}
		labels := map[string]int{}
		for _, s := range spans {
			labels[markup.UnescapeHTML(s.Content)]++
		}
		sort := r.configuration.Tag.Sort
		if sortMode != nil {
			sort = *sortMode
		}
		ret.Tags, ret.Exceeded, e = tagquery.Build(ctx, labels, sort, r.configuration.FileTree.MaxListCount, ignoreLimit)
		return e
	})
	if err != nil {
		return NativeTagTree{}, err
	}
	return
}
