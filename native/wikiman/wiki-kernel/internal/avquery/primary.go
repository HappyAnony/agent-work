// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model AV primary query, with explicit owner dependencies.
package avquery

import (
	"context"
	"errors"
	"sort"
	"strings"
)

var ErrPrimaryPage = errors.New("invalid_av_primary_page")
var ErrViewNotFound = errors.New("view not found")

type PrimaryMeta struct {
	BoundID, Text string
	Updated       int64
	Detached      bool
}

func BlockText(content string) string { return strings.TrimSpace(content) }

func PrimaryValues[T any](ctx context.Context, values []T, describe func(T) (PrimaryMeta, error), exists func(context.Context, string) (bool, error), keyword string, page, pageSize int) (ret []T, err error) {
	if page < 1 {
		return nil, ErrPrimaryPage
	}
	if pageSize < 1 {
		pageSize = 16
	}
	if describe == nil || exists == nil {
		return nil, errors.New("av_primary_dependency_unbound")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	type entry struct {
		Value T
		Meta  PrimaryMeta
	}
	var matches []entry
	for _, value := range values {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		meta, e := describe(value)
		if e != nil {
			return nil, e
		}
		if !meta.Detached {
			ok, e := exists(ctx, meta.BoundID)
			if e != nil {
				return nil, e
			}
			if !ok {
				continue
			}
		}
		if strings.Contains(strings.ToLower(meta.Text), strings.ToLower(keyword)) {
			matches = append(matches, entry{Value: value, Meta: meta})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Meta.Updated > matches[j].Meta.Updated })
	// The old unchecked slice panicked beyond the last page or on overflow.
	// Valid pages retain the original order/default size; out-of-range is empty.
	if page-1 > len(matches)/pageSize {
		return nil, ctx.Err()
	}
	start := (page - 1) * pageSize
	if start >= len(matches) {
		return nil, ctx.Err()
	}
	end := start + min(pageSize, len(matches)-start)
	for _, m := range matches[start:end] {
		ret = append(ret, m.Value)
	}
	return ret, ctx.Err()
}

func CurrentView[T any](ctx context.Context, views []T, requested, current string, id func(T) string) (zero T, err error) {
	if id == nil {
		return zero, errors.New("av_view_dependency_unbound")
	}
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	if requested != "" {
		for _, view := range views {
			if err = ctx.Err(); err != nil {
				return zero, err
			}
			if id(view) == requested {
				return view, nil
			}
		}
	}
	for _, view := range views {
		if err = ctx.Err(); err != nil {
			return zero, err
		}
		if id(view) == current {
			return view, nil
		}
	}
	if len(views) == 0 {
		return zero, ErrViewNotFound
	}
	return views[0], ctx.Err()
}

func MirrorIDs(ctx context.Context, ids []string, exists func(context.Context, string) (bool, error)) (ret []string, err error) {
	if exists == nil {
		return nil, errors.New("av_mirror_dependency_unbound")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		ok, e := exists(ctx, id)
		if e != nil {
			return nil, e
		}
		if ok {
			seen[id] = true
		}
	}
	ret = []string{}
	for id := range seen {
		ret = append(ret, id)
	}
	return ret, ctx.Err()
}
