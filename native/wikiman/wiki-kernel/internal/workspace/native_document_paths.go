package workspace

import (
	"context"
	"path"
	"slices"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

// No global tree, source repair or implicit main-index flush. The blocktree
// registry is an owner-local derived projection and is built only when needed.
func (l *Lease) NativeDocumentPaths(ctx context.Context, q docquery.PathRequest) (ret docquery.PathResult, err error) {
	q.Paths = slices.Clone(q.Paths)
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		query := func(ctx context.Context, f indexdb.BlockTreeFilter) ([]indexdb.BlockTreeRecord, error) {
			if !r.blockTreeReady {
				if e := r.rebuildBlockTree(ctx, w); e != nil {
					return nil, e
				}
			}
			db, e := b.Database(indexdb.BlockTree)
			if e != nil {
				return nil, e
			}
			return (indexdb.BlockTrees{DB: db}).Query(ctx, f)
		}
		boxName := func(ctx context.Context, id string) (string, bool, error) {
			boxes, e := r.configurationBusiness.Notebooks(ctx)
			if e != nil {
				return "", false, e
			}
			for _, box := range boxes {
				if box.ID == id {
					return box.Name, true, nil
				}
			}
			return "", false, nil
		}
		services := docquery.PathServices{
			LoadPath: func(ctx context.Context, box, p string) (docquery.Location, error) {
				if !documentID.MatchString(box) || !strings.HasPrefix(p, "/") || !strings.HasSuffix(p, ".docj") {
					return docquery.Location{}, ErrDocument
				}
				rel, e := treeRelative(box, p)
				if e != nil {
					return docquery.Location{}, e
				}
				id := strings.TrimSuffix(path.Base(p), ".docj")
				if !documentID.MatchString(id) {
					return docquery.Location{}, ErrDocument
				}
				raw, e := readNativeFile(ctx, w, rel, 16<<20)
				if e != nil {
					return docquery.Location{}, e
				}
				t, e := r.parseOriginalTree(ctx, w, cachedDocument{raw: raw, path: p}, box, id)
				if e != nil {
					return docquery.Location{}, e
				}
				if t.Root.ID != id {
					return docquery.Location{}, ErrDocument
				}
				t, e = locateOriginalTree(ctx, w, t, box)
				if e != nil {
					return docquery.Location{}, e
				}
				return docquery.Location{Path: t.Path, Box: t.Box, HPath: t.HPath}, nil
			},
			LoadID: func(ctx context.Context, id string) (docquery.Location, error) {
				rows, e := query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
				if e != nil {
					return docquery.Location{}, e
				}
				if len(rows) == 0 {
					return docquery.Location{}, ErrDocument
				}
				t, e := r.locatedTree(ctx, w, rows[0].BoxID, rows[0].RootID)
				if e != nil {
					return docquery.Location{}, e
				}
				return docquery.Location{Path: t.Path, Box: t.Box, HPath: t.HPath}, nil
			},
			BoxName: boxName,
			BoxesByPaths: func(ctx context.Context, paths []string) (map[string]*conf.Notebook, error) {
				ret := map[string]*conf.Notebook{}
				for _, p := range paths {
					rows, e := query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: strings.TrimSuffix(path.Base(p), ".docj")})
					if e != nil {
						return nil, e
					}
					if len(rows) == 0 {
						continue
					}
					row := rows[0]
					name, found, e := boxName(ctx, row.BoxID)
					if e != nil {
						return nil, e
					}
					if found {
						ret[row.Path] = &conf.Notebook{ID: row.BoxID, Name: name}
					} else {
						ret[row.Path] = nil
					}
				}
				return ret, nil
			},
			BlockByPath: func(ctx context.Context, p string) (*indexdb.BlockTreeRecord, error) {
				rows, e := query(ctx, indexdb.BlockTreeFilter{Kind: "path", Value: p})
				if e != nil {
					return nil, e
				}
				if len(rows) == 0 {
					return nil, nil
				}
				return &rows[0], nil
			},
			Roots: func(ctx context.Context, box, hpath string) ([]string, error) {
				rows, e := query(ctx, indexdb.BlockTreeFilter{Kind: "root-hpath", BoxID: box, Value: hpath})
				if e != nil {
					return nil, e
				}
				ids := []string{}
				for _, row := range rows {
					ids = append(ids, row.ID)
				}
				return ids, nil
			},
			MissingBox: func() error { return docquery.ErrPathBoxMissing },
		}
		var e error
		ret, e = docquery.Paths(ctx, q, services)
		return e
	})
	if err != nil {
		return docquery.PathResult{}, err
	}
	return
}
