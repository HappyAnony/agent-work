// SPDX-License-Identifier: AGPL-3.0-or-later
package tagquery

import (
	"context"
	"errors"
	"sort"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
)

type NavigationServices struct {
	Spans  func(context.Context, string, int) ([]indexdb.SpanRecord, error)
	Blocks func(context.Context, []string) ([]*indexdb.BlockRecord, error)
}

// Navigate retains span order and duplicate hits per label. Only the IDs passed
// to the block reader are deduplicated and sorted descending, as in the original
// labelBlocksByKeyword. A disappeared block is represented by a nil list entry.
func Navigate(ctx context.Context, keyword string, limit int, caseSensitive bool, s NavigationServices) (map[string][]*blockview.Block, error) {
	if ctx == nil || s.Spans == nil || s.Blocks == nil {
		return nil, errors.New("tag_navigation_dependencies_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	spans, err := s.Spans(ctx, keyword, limit)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, span := range spans {
		set[span.BlockID] = true
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	rows, err := s.Blocks(ctx, ids)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	byID := map[string]*indexdb.BlockRecord{}
	for _, row := range rows {
		if row != nil {
			byID[row.ID] = row
		}
	}
	ret := map[string][]*blockview.Block{}
	for _, span := range spans {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		ret[span.Content] = append(ret[span.Content], blockview.FromRow(byID[span.BlockID], "", 0, caseSensitive))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}
