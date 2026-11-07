package model

import (
	"context"
	"errors"
	"fmt"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/tagquery"
	"wiki-kernel/util"
)

type tagReadContext struct {
	r                *indexRepairContext
	sort, max, limit int
	caseSensitive    bool
	message          string
}

func captureTagRead(ctx context.Context, sortMode *int) (*tagReadContext, error) {
	if ctx == nil {
		return nil, errors.New("tag_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r, err := captureIndexRepair(ctx)
	if err != nil {
		return nil, err
	}
	o := &tagReadContext{r: r}
	err = r.owner.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		if err := r.guard(ctx); err != nil {
			return err
		}
		if data.Tag == nil || data.FileTree == nil || data.Search == nil {
			return conf.ErrBusinessUnbound
		}
		if sortMode != nil {
			data.Tag.Sort = *sortMode
			r.owner.m.Lock()
			if r.owner.persistenceReadOnly {
				r.owner.m.Unlock()
				return ErrSettingReadOnly
			}
			err := r.owner.saveDataLocked(data)
			if err == nil {
				r.owner.AppData = data
			}
			r.owner.m.Unlock()
			if err != nil {
				return err
			}
		}
		o.sort, o.max, o.limit, o.caseSensitive = data.Tag.Sort, data.FileTree.MaxListCount, data.Search.Limit, data.Search.CaseSensitive
		o.message = r.language(243)
		return nil
	})
	return o, err
}

func BuildTagsContext(ctx context.Context, sortMode *int, ignoreLimit bool, app string) (*Tags, error) {
	o, err := captureTagRead(ctx, sortMode)
	if err != nil {
		return nil, err
	}
	if err = FlushTxQueueContext(ctx); err != nil {
		return nil, err
	}
	if err = o.r.queue.Flush(ctx); err != nil {
		return nil, err
	}
	spans, err := indexdb.TagSpans(ctx, o.r.main.DB, "path", "", 0)
	if err != nil {
		return nil, err
	}
	labels := map[string]int{}
	for _, span := range spans {
		labels[util.UnescapeHTML(span.Content)]++
	}
	tree, exceeded, err := tagquery.Build(ctx, labels, o.sort, o.max, ignoreLimit)
	if err != nil {
		return nil, err
	}
	if err = o.r.guard(ctx); err != nil {
		return nil, err
	}
	if exceeded {
		o.r.owner.publishSyncUI(ctx, func() { util.PushMsgWithApp(app, fmt.Sprintf(o.message, o.max), 7000) })
	}
	var convert func(tagquery.Tags) Tags
	convert = func(src tagquery.Tags) Tags {
		if src == nil {
			return nil
		}
		out := Tags{}
		for _, n := range src {
			out = append(out, &Tag{Name: n.Name, Label: n.Label, Type: n.Type, Depth: n.Depth, Count: n.Count, Children: convert(n.Children)})
		}
		return out
	}
	result := convert(tree)
	return &result, nil
}

func SearchTagsContext(ctx context.Context, keyword string) ([]string, error) {
	o, err := captureTagRead(ctx, nil)
	if err != nil {
		return nil, err
	}
	if err = o.r.queue.Flush(ctx); err != nil {
		return nil, err
	}
	result, err := indexdb.SearchTagLabels(ctx, o.r.main.DB, keyword, o.limit, o.caseSensitive)
	if err != nil {
		return nil, err
	}
	if err = o.r.guard(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
