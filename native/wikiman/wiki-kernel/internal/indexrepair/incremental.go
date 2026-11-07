// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"errors"
	"io/fs"
	"strings"

	"github.com/88250/lute/ast"
)

type DocumentPath struct{ Relative, Box, Path, ID string }

// DocumentPaths retains one optional leading slash, input order and metadata /
// orphan-file skipping. Validate the complete document plan before any mutation.
func DocumentPaths(paths []string) (ret []DocumentPath, err error) {
	for _, name := range paths {
		if !strings.HasSuffix(name, ".docj") {
			continue
		}
		name = strings.TrimPrefix(name, "/")
		if !fs.ValidPath(name) || strings.Contains(name, "\\") {
			return nil, ErrSourcePath
		}
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			continue
		} // Old data-root orphans never belong to a notebook.
		if !ast.IsNodeIDPattern(parts[0]) || strings.Contains("/"+name, "/assets/") || strings.Contains("/"+name, "/.siyuan/") {
			continue
		} // Attachments / metadata do not become notebooks.
		for i, part := range parts {
			if i == len(parts)-1 {
				part = strings.TrimSuffix(part, ".docj")
			}
			if !ast.IsNodeIDPattern(part) {
				return nil, ErrSourcePath
			}
		}
		ret = append(ret, DocumentPath{Relative: name, Box: parts[0], Path: "/" + strings.Join(parts[1:], "/"), ID: strings.TrimSuffix(parts[len(parts)-1], ".docj")})
	}
	return ret, nil
}

type IncrementalResult struct{ UpsertRootIDs, RemoveRootIDs []string }
type IncrementalServices struct {
	Remove func(context.Context, DocumentPath) error
	Upsert func(context.Context, DocumentPath) error
	Flush  func(context.Context) error
}

var ErrIncrementalDependency = errors.New("incremental_index_unbound")

// Incremental preserves remove-before-upsert, including a flush boundary so a
// same-ID move cannot be masked by stale derived data. Returned IDs describe a
// successful fully drained run only. Errors never imply source/derived rollback.
func Incremental(ctx context.Context, upserts, removes []string, s IncrementalServices) (out IncrementalResult, err error) {
	if s.Remove == nil || s.Upsert == nil || s.Flush == nil {
		return out, ErrIncrementalDependency
	}
	ups, e := DocumentPaths(upserts)
	if e != nil {
		return out, e
	}
	rms, e := DocumentPaths(removes)
	if e != nil {
		return out, e
	}
	dirty := false
	drain := func() (e error) {
		defer func() {
			if recover() != nil {
				e = errors.New("incremental_drain_failed")
			}
		}()
		return s.Flush(context.WithoutCancel(ctx))
	}
	defer func() {
		if recover() != nil {
			err = errors.New("incremental_index_failed")
		}
		if dirty {
			err = errors.Join(err, drain())
		}
		if err != nil {
			out = IncrementalResult{}
		}
	}()
	out = IncrementalResult{UpsertRootIDs: []string{}, RemoveRootIDs: []string{}}
	for phase, paths := range [][]DocumentPath{rms, ups} {
		if err = ctx.Err(); err != nil {
			return
		}
		dirty = true // Also drain previously accepted work on a failed callback.
		for _, p := range paths {
			if err = ctx.Err(); err != nil {
				return
			}
			if phase == 0 {
				err = s.Remove(ctx, p)
			} else {
				err = s.Upsert(ctx, p)
			}
			if err != nil {
				return
			}
			if phase == 0 {
				out.RemoveRootIDs = append(out.RemoveRootIDs, p.ID)
			} else {
				out.UpsertRootIDs = append(out.UpsertRootIDs, p.ID)
			}
		}
		err = drain()
		dirty = false
		if err != nil {
			return
		}
	}
	err = ctx.Err()
	return
}
