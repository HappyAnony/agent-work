// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"path"
	"slices"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func DocumentPathsContext(ctx context.Context, q docquery.PathRequest) (docquery.PathResult, error) {
	q.Paths = slices.Clone(q.Paths)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (docquery.PathResult, error) {
		query := func(c context.Context, f indexdb.BlockTreeFilter) ([]indexdb.BlockTreeRecord, error) {
			if e := o.r.guard(c); e != nil {
				return nil, e
			}
			return o.r.registry.Query(c, f)
		}
		boxes, e := o.base.owner.business.Notebooks(c)
		if e != nil {
			return docquery.PathResult{}, e
		}
		names := map[string]conf.Notebook{}
		for _, box := range boxes {
			if !box.Closed {
				names[box.ID] = box
			}
		}
		return docquery.Paths(c, q, docquery.PathServices{
			LoadPath: func(c context.Context, box, p string) (docquery.Location, error) {
				tree, e := o.r.loadRepairTree(c, box, p, o.parser)
				if e != nil {
					return docquery.Location{}, e
				}
				return pathLocation(tree), nil
			},
			LoadID: func(c context.Context, id string) (docquery.Location, error) {
				tree, e := o.load(c, id)
				if e != nil {
					return docquery.Location{}, e
				}
				return pathLocation(tree), nil
			},
			BoxName: func(c context.Context, id string) (string, bool, error) {
				box, ok := names[id]
				return box.Name, ok, c.Err()
			},
			BoxesByPaths: func(c context.Context, paths []string) (map[string]*conf.Notebook, error) {
				out := map[string]*conf.Notebook{}
				for _, p := range paths {
					id := strings.TrimSuffix(path.Base(p), path.Ext(p))
					rows, e := query(c, indexdb.BlockTreeFilter{Kind: "id", Value: id})
					if e != nil {
						return nil, e
					}
					if len(rows) == 0 {
						continue
					}
					box, ok := names[rows[0].BoxID]
					if ok {
						copy := box
						out[rows[0].Path] = &copy
					} else {
						out[rows[0].Path] = nil
					}
				}
				return out, c.Err()
			},
			BlockByPath: func(c context.Context, p string) (*indexdb.BlockTreeRecord, error) {
				rows, e := query(c, indexdb.BlockTreeFilter{Kind: "path", Value: p})
				if e != nil || len(rows) == 0 {
					return nil, e
				}
				return &rows[0], nil
			},
			Roots: func(c context.Context, box, p string) ([]string, error) {
				rows, e := query(c, indexdb.BlockTreeFilter{Kind: "root-hpath", BoxID: box, Value: p})
				if e != nil {
					return nil, e
				}
				ids := []string{}
				for _, row := range rows {
					ids = append(ids, row.ID)
				}
				return ids, nil
			},
			MissingBox: func() error { return ErrBoxNotFound },
		})
	})
}
func GetHPathByPathContext(ctx context.Context, box, p string) (string, error) {
	v, e := DocumentPathsContext(ctx, docquery.PathRequest{Mode: "path", Box: box, Path: p})
	return v.HPath, e
}
func GetHPathsByPathsContext(ctx context.Context, paths []string) ([]string, error) {
	v, e := DocumentPathsContext(ctx, docquery.PathRequest{Mode: "batch", Paths: paths})
	return v.Paths, e
}
func GetHPathByIDContext(ctx context.Context, id string) (string, error) {
	v, e := DocumentPathsContext(ctx, docquery.PathRequest{Mode: "id", ID: id})
	return v.HPath, e
}
func GetFullHPathByIDContext(ctx context.Context, id string) (string, error) {
	v, e := DocumentPathsContext(ctx, docquery.PathRequest{Mode: "full", ID: id})
	return v.HPath, e
}
func GetPathByIDContext(ctx context.Context, id string) (string, string, error) {
	v, e := DocumentPathsContext(ctx, docquery.PathRequest{Mode: "location", ID: id})
	return v.Path, v.Box, e
}
func GetIDsByHPathContext(ctx context.Context, p, box string) ([]string, error) {
	v, e := DocumentPathsContext(ctx, docquery.PathRequest{Mode: "ids", Box: box, HPath: p})
	return v.IDs, e
}
