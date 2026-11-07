// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package docquery

import (
	"context"
	"errors"
	"html"
	"slices"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

type PathRequest struct {
	Mode, ID, Box, Path, HPath string
	Paths                      []string
}
type Location struct{ Path, Box, HPath string }
type PathResult struct {
	Location
	Paths, IDs []string
}
type PathServices struct {
	LoadPath     func(context.Context, string, string) (Location, error)
	LoadID       func(context.Context, string) (Location, error)
	BoxName      func(context.Context, string) (string, bool, error)
	BoxesByPaths func(context.Context, []string) (map[string]*conf.Notebook, error)
	BlockByPath  func(context.Context, string) (*indexdb.BlockTreeRecord, error)
	Roots        func(context.Context, string, string) ([]string, error)
	MissingBox   func() error
	Warn         func(string, string)
}

var ErrPathBoxMissing = errors.New("document_path_box_missing")

// Single lookups use the current tree; batch lookups retain the original
// indexed/unescaped display path, dedup by actual path and unspecified order.
func Paths(ctx context.Context, q PathRequest, s PathServices) (ret PathResult, err error) {
	zero := PathResult{}
	if ctx == nil {
		return zero, errors.New("document_path_dependencies_unbound")
	}
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	guard := func(fn func() error) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := fn(); e != nil {
			return e
		}
		return ctx.Err()
	}
	switch q.Mode {
	case "path":
		if q.Path == "/" {
			ret.HPath = "/"
			return ret, nil
		}
		if s.LoadPath == nil {
			return zero, errors.New("document_path_dependencies_unbound")
		}
		err = guard(func() (e error) { ret.Location, e = s.LoadPath(ctx, q.Box, q.Path); return })
		if err != nil {
			return zero, err
		}
	case "id", "location", "full":
		if s.LoadID == nil || (q.Mode == "full" && (s.BoxName == nil || s.MissingBox == nil)) {
			return zero, errors.New("document_path_dependencies_unbound")
		}
		if err = guard(func() (e error) { ret.Location, e = s.LoadID(ctx, q.ID); return }); err != nil {
			return zero, err
		}
		if q.Mode == "full" {
			var name string
			var found bool
			if err = guard(func() (e error) { name, found, e = s.BoxName(ctx, ret.Box); return }); err != nil {
				return zero, err
			}
			if !found {
				if err = guard(s.MissingBox); err == nil {
					err = ErrPathBoxMissing
				}
				return zero, err
			}
			ret.HPath = name + ret.HPath
		}
	case "batch":
		if s.BoxesByPaths == nil || s.BlockByPath == nil {
			return zero, errors.New("document_path_dependencies_unbound")
		}
		input := slices.Clone(q.Paths)
		var boxes map[string]*conf.Notebook
		if err = guard(func() (e error) { boxes, e = s.BoxesByPaths(ctx, input); return }); err != nil {
			return zero, err
		}
		copy := map[string]*conf.Notebook{}
		for p, b := range boxes {
			if b != nil {
				v := *b
				copy[p] = &v
			} else {
				copy[p] = nil
			}
		}
		for p, b := range copy {
			if err = ctx.Err(); err != nil {
				return zero, err
			}
			if b == nil {
				if s.Warn != nil {
					s.Warn("box", p)
				}
				continue
			}
			var row *indexdb.BlockTreeRecord
			if err = guard(func() (e error) { row, e = s.BlockByPath(ctx, p); return }); err != nil {
				return zero, err
			}
			if row == nil {
				if s.Warn != nil {
					s.Warn("tree", p)
				}
				continue
			}
			ret.Paths = append(ret.Paths, b.Name+html.UnescapeString(row.HPath))
		}
	case "ids":
		if s.Roots == nil {
			return zero, errors.New("document_path_dependencies_unbound")
		}
		var ids []string
		if err = guard(func() (e error) { ids, e = s.Roots(ctx, q.Box, q.HPath); return }); err != nil {
			return zero, err
		}
		ret.IDs = []string{}
		seen := map[string]bool{}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				ret.IDs = append(ret.IDs, id)
			}
		}
	default:
		return zero, errors.New("invalid_document_path_mode")
	}
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	return ret, nil
}
