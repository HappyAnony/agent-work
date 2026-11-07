package workspace

import (
	"context"
	"crypto/sha256"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

// PlanNativeDocumentLifecycle prepares detached original-format candidates and
// the complete effect manifest. It grants no source/history commit authority.
// HistoryPath is relative to this owner's runtime history input, never an
// arbitrary host path. R04/R05 will bind the authoritative publication protocol.
func (l *Lease) PlanNativeDocumentLifecycle(ctx context.Context, q doclife.Request) (ret doclife.Plan, err error) {
	return l.planNativeDocumentLifecycleWithIO(ctx, q, workspaceDirectoryIO{})
}

func (l *Lease) planNativeDocumentLifecycleWithIO(ctx context.Context, q doclife.Request, op workspaceDirectoryIO) (ret doclife.Plan, err error) {
	q.Paths = slices.Clone(q.Paths)
	q.Sorts = slices.Clone(q.Sorts)
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		services, inputs, guard, e := l.workspace.resources.documentLifecycleServicesWithIO(ctx, l.workspace, b, q.Box, op)
		if e != nil {
			return e
		}
		ret, e = doclife.Prepare(ctx, q, services)
		if e == nil {
			e = guard.Confirm(ctx)
		}
		if e == nil {
			ret.SourceHashes = inputs
		}
		return e
	})
	if err != nil {
		return doclife.Plan{}, err
	}
	return
}

func (r *resources) documentLifecycleServices(ctx context.Context, w *WorkspaceContext, b *indexdb.Bundle, historyBox string) (doclife.Services, map[string][32]byte, *nativeDocumentLifecycleEnumerationGuard, error) {
	return r.documentLifecycleServicesWithIO(ctx, w, b, historyBox, workspaceDirectoryIO{})
}

func (r *resources) documentLifecycleServicesWithIO(ctx context.Context, w *WorkspaceContext, b *indexdb.Bundle, historyBox string, op workspaceDirectoryIO) (doclife.Services, map[string][32]byte, *nativeDocumentLifecycleEnumerationGuard, error) {
	inputs := map[string][32]byte{}
	if r.configuration.FileTree == nil || r.configurationBusiness == nil {
		return doclife.Services{}, nil, nil, conf.ErrBusinessUnbound
	}
	if !r.blockTreeReady {
		if e := r.rebuildBlockTree(ctx, w); e != nil {
			return doclife.Services{}, nil, nil, e
		}
	}
	db, e := b.Database(indexdb.BlockTree)
	if e != nil {
		return doclife.Services{}, nil, nil, e
	}
	guard := newNativeDocumentLifecycleEnumerationGuard(w, op)
	query := indexdb.BlockTrees{DB: db}
	lookup := func(ctx context.Context, id string) (doclife.Location, bool, error) {
		if !documentID.MatchString(id) {
			return doclife.Location{}, false, nil
		}
		rows, e := query.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
		if e != nil || len(rows) == 0 {
			return doclife.Location{}, false, e
		}
		v := rows[0]
		return doclife.Location{ID: v.ID, Box: v.BoxID, Path: v.Path, HPath: v.HPath, Document: v.Type == "d"}, true, nil
	}
	load := func(ctx context.Context, box, p string) (*parse.Tree, error) {
		rel, e := treeRelative(box, p)
		if e != nil {
			return nil, e
		}
		id := strings.TrimSuffix(path.Base(p), ".docj")
		if !documentID.MatchString(id) {
			return nil, ErrDocument
		}
		raw, e := readNativeFile(ctx, w, rel, 16<<20)
		if e != nil {
			return nil, e
		}
		inputs["data/"+rel] = sha256.Sum256(raw)
		t, e := r.parseOriginalTree(ctx, w, cachedDocument{raw: raw, path: p}, box, id)
		if e != nil {
			return nil, e
		}
		if t.Root.ID != id {
			return nil, ErrDocument
		}
		return locateOriginalTree(ctx, w, t, box)
	}
	services := doclife.Services{
		AllowDeeper: r.configuration.FileTree.AllowCreateDeeper,
		Box: func(ctx context.Context, id string) (bool, error) {
			boxes, e := r.configurationBusiness.Notebooks(ctx)
			if e != nil {
				return false, e
			}
			for _, box := range boxes {
				if box.ID == id {
					return true, nil
				}
			}
			return false, ctx.Err()
		},
		Load: load, Lookup: lookup,
		Children: guard.children,
		Exists: func(ctx context.Context, box, p string) (bool, error) {
			rel, e := treeRelative(box, p)
			if e != nil {
				return false, e
			}
			f, e := openNativeFile(w, rel)
			if os.IsNotExist(e) {
				return false, nil
			}
			if e != nil {
				return false, e
			}
			f.Close()
			return true, ctx.Err()
		},
		Parser: func(ctx context.Context) (*lute.Lute, error) {
			opts, e := r.configuredParserOptions(ctx, w)
			if e != nil {
				return nil, e
			}
			return nativeparser.NewEditor(opts), ctx.Err()
		},
		History: func(ctx context.Context, p string) (*parse.Tree, error) {
			if !fs.ValidPath(p) || filepath.IsAbs(p) {
				return nil, ErrDocument
			}
			historyRoot := filepath.Join(w.paths.RuntimeDir, "history")
			info, e := os.Lstat(historyRoot)
			if os.IsNotExist(e) {
				return nil, nil
			}
			if e != nil {
				return nil, e
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return nil, ErrDocument
			}
			root, e := os.OpenRoot(historyRoot)
			if os.IsNotExist(e) {
				return nil, nil
			}
			if e != nil {
				return nil, e
			}
			defer root.Close()
			for i := range strings.Split(p, "/") {
				parts := strings.Split(p, "/")
				info, e := root.Lstat(strings.Join(parts[:i+1], "/"))
				if os.IsNotExist(e) {
					return nil, nil
				}
				if e != nil {
					return nil, e
				}
				if info.Mode()&fs.ModeSymlink != 0 {
					return nil, ErrDocument
				}
			}
			f, e := root.Open(p)
			if e != nil {
				return nil, e
			}
			defer f.Close()
			raw, e := readLifecycleHistory(f)
			if e != nil {
				return nil, e
			}
			inputs["history/"+p] = sha256.Sum256(raw)
			id := strings.TrimSuffix(path.Base(p), ".docj")
			if !documentID.MatchString(id) {
				return nil, ErrDocument
			}
			t, e := r.parseOriginalTree(ctx, w, cachedDocument{raw: raw, path: "/" + id + ".docj"}, historyBox, id)
			if e != nil {
				return nil, e
			}
			if t.Root.ID != id {
				return nil, ErrDocument
			}
			return t, ctx.Err()
		},
		ExistingIDs: func(ctx context.Context, ids []string, exclude string) (map[string]bool, error) {
			ret := map[string]bool{}
			for _, id := range ids {
				rows, e := query.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
				if e != nil {
					return nil, e
				}
				for _, row := range rows {
					if row.RootID != exclude {
						ret[id] = true
					}
				}
			}
			return ret, ctx.Err()
		},
		Language: func(code int) string {
			return r.configurationBusiness.Language(r.configuration.Lang, code, true, r.configuration.CloudRegion)
		}, PlainLanguage: func(code int) string {
			return r.configurationBusiness.Language(r.configuration.Lang, code, false, r.configuration.CloudRegion)
		}, Now: func() string { return time.Now().Format("20060102150405") }, NewID: ast.NewNodeID,
	}
	return services, inputs, guard, nil
}

func readLifecycleHistory(f *os.File) ([]byte, error) {
	info, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, ErrDocument
	}
	raw, e := io.ReadAll(io.LimitReader(f, 16<<20+1))
	if e != nil {
		return nil, e
	}
	if len(raw) > 16<<20 {
		return nil, ErrBudget
	}
	return raw, nil
}
