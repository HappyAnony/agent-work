package workspace

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

// All loaders run under the caller's source admission/index gate. Missing
// registry IDs are skipped in batches, but known corrupt/missing source fails.
func (r *resources) documentInfoTrees(ctx context.Context, w *WorkspaceContext, ids []string) (map[string]*parse.Tree, error) {
	if !r.blockTreeReady {
		if e := r.rebuildBlockTree(ctx, w); e != nil {
			return nil, e
		}
	}
	db, e := r.native.Database(indexdb.BlockTree)
	if e != nil {
		return nil, e
	}
	ret := map[string]*parse.Tree{}
	loaded := map[string]*parse.Tree{}
	for _, id := range ids {
		if !documentID.MatchString(id) {
			continue
		}
		rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
		if e != nil {
			return nil, e
		}
		if len(rows) == 0 {
			continue
		}
		row := rows[0]
		tree := loaded[row.RootID]
		if tree == nil {
			tree, e = r.locatedTree(ctx, w, row.BoxID, row.RootID)
			if e != nil {
				return nil, e
			}
			loaded[row.RootID] = tree
		}
		ret[id] = tree
	}
	return ret, ctx.Err()
}
func (r *resources) documentInfoParser(ctx context.Context, w *WorkspaceContext) (*lute.Lute, error) {
	options, e := r.configuredParserOptions(ctx, w)
	if e != nil {
		return nil, e
	}
	return nativeparser.NewEditor(options), ctx.Err()
}
func (r *resources) nativeReferenceDefinitions(ctx context.Context, w *WorkspaceContext, id, root string) ([]*docquery.RefDefs, map[string]string, error) {
	db, e := r.native.Database(indexdb.Main)
	if e != nil {
		return nil, nil, e
	}
	q := indexdb.References{DB: db}
	var defs []*docquery.RefDefs
	if id == root {
		refs, e := q.ChildDefinitions(ctx, root)
		if e != nil {
			return nil, nil, e
		}
		for ref, ids := range refs {
			defs = append(defs, &docquery.RefDefs{RefID: ref, DefIDs: slices.Clone(ids)})
		}
	} else {
		ids, e := q.RefIDs(ctx, id, false)
		if e != nil {
			return nil, nil, e
		}
		for _, ref := range ids {
			defs = append(defs, &docquery.RefDefs{RefID: ref, DefIDs: []string{id}})
		}
	}
	if r.configuration.Search == nil {
		return nil, nil, conf.ErrBusinessUnbound
	}
	original, e := docquery.PromoteRefDefs(ctx, defs, docquery.RefPromotionServices{CaseSensitive: r.configuration.Search.CaseSensitive, Parser: func(ctx context.Context) (*lute.Lute, error) { return r.documentInfoParser(ctx, w) }, Blocks: func(ctx context.Context, ids []string) (ret []indexdb.BlockRecord, err error) {
		for _, id := range ids {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			b, ok, e := r.nativeBlock(ctx, id)
			if e != nil {
				return nil, e
			}
			if ok {
				ret = append(ret, b)
			}
		}
		return ret, nil
	}})
	if e != nil {
		return nil, nil, e
	}
	return defs, original, ctx.Err()
}

func (r *resources) documentInfoReferences(ctx context.Context, w *WorkspaceContext, id, root string) ([]string, error) {
	defs, _, err := r.nativeReferenceDefinitions(ctx, w, id, root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, d := range defs {
		ids = append(ids, d.RefID)
	}
	return ids, ctx.Err()
}

type NativeReferenceDefinitions struct {
	Definitions []*docquery.RefDefs `json:"refIDs"`
	Original    map[string]string   `json:"originalRefIDs"`
}

func (l *Lease) NativeBlockReferences(ctx context.Context, id string) (out NativeReferenceDefinitions, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		tree, e := r.nativeTreeByBlock(ctx, w, id)
		if e != nil {
			return e
		}
		out.Definitions, out.Original, e = r.nativeReferenceDefinitions(ctx, w, id, tree.Root.ID)
		return e
	})
	if err != nil {
		return NativeReferenceDefinitions{}, err
	}
	return
}

func (l *Lease) NativeDocInfo(ctx context.Context, id string) (*docquery.Info, error) {
	rows, e := l.nativeDocumentInfos(ctx, docquery.InfoRequest{IDs: []string{id}})
	if e != nil {
		return nil, e
	}
	if len(rows) == 0 {
		return nil, ErrDocument
	}
	return rows[0], nil
}
func (l *Lease) NativeDocsInfo(ctx context.Context, ids []string, refs, avs bool) ([]*docquery.Info, error) {
	return l.nativeDocumentInfos(ctx, docquery.InfoRequest{IDs: ids, Batch: true, References: refs, AV: avs})
}
func (l *Lease) nativeDocumentInfos(ctx context.Context, q docquery.InfoRequest) (ret []*docquery.Info, err error) {
	q.IDs = slices.Clone(q.IDs)
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		return nil, err
	}
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		var e error
		ret, e = docquery.Infos(ctx, q, docquery.InfoServices{
			Wait: func(ctx context.Context) error { return ctx.Err() }, Trees: func(ctx context.Context, ids []string) (map[string]*parse.Tree, error) {
				return r.documentInfoTrees(ctx, w, ids)
			},
			Exists: func(ctx context.Context, id string) (bool, error) {
				if !documentID.MatchString(id) {
					return false, nil
				}
				db, e := r.native.Database(indexdb.BlockTree)
				if e != nil {
					return false, e
				}
				rows, e := (indexdb.BlockTrees{DB: db}).Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
				return len(rows) > 0, e
			},
			References: func(ctx context.Context, id, root string) ([]string, error) {
				return r.documentInfoReferences(ctx, w, id, root)
			},
			AVName: func(ctx context.Context, id string) (string, error) {
				name, _, e := r.readNativeAV(ctx, w, id)
				return name, e
			},
			DefaultAVName: func(ctx context.Context) (string, error) {
				if r.configurationBusiness == nil {
					return "", conf.ErrBusinessUnbound
				}
				name := r.configurationBusiness.Language(r.configuration.Lang, 105, false, r.configuration.CloudRegion)
				if name == "" {
					return "", conf.ErrBusinessUnbound
				}
				return name, ctx.Err()
			},
			Subfiles: func(ctx context.Context, t *parse.Tree) (int, error) {
				rel, e := treeRelative(t.Box, strings.TrimSuffix(t.Path, ".docj"))
				if e != nil {
					return 0, e
				}
				info, e := treeFileStat(ctx, w, rel)
				if errors.Is(e, fs.ErrNotExist) && !q.Batch {
					return 0, nil
				}
				if e != nil {
					return 0, e
				}
				if !info.IsDir() {
					return 0, ErrDocument
				}
				manifest, e := confirmedWorkspaceDirectoryManifest(ctx, w, rel, workspaceDirectoryIO{})
				if e != nil {
					return 0, e
				}
				count := 0
				for _, f := range manifest.entries {
					if e = ctx.Err(); e != nil {
						return 0, e
					}
					if f.info.Mode()&fs.ModeSymlink != 0 {
						return 0, ErrDocument
					}
					if strings.HasSuffix(path.Base(f.name), ".docj") {
						count++
					}
				}
				return count, nil
			},
		})
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

// Statistics do not acquire an implicit main-index flush (same as the original
// filesys entry points). Tree registry, source/parser and AV all share one gate.
func (l *Lease) NativeDocumentStatistics(ctx context.Context, q docquery.StatRequest) (ret *docquery.StatResult, err error) {
	q.IDs = slices.Clone(q.IDs)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		var e error
		ret, e = docquery.Statistics(ctx, q, docquery.StatServices{Trees: func(ctx context.Context, ids []string) (map[string]*parse.Tree, error) {
			return r.documentInfoTrees(ctx, w, ids)
		}, Parser: func(ctx context.Context) (*lute.Lute, error) { return r.documentInfoParser(ctx, w) }, AttributeView: func(ctx context.Context, id string) (*avmodel.AttributeView, error) {
			raw, _, e := readNativeAVSource(ctx, w, id)
			if e != nil {
				return nil, e
			}
			return decodeNativeAV(raw, id, time.Now())
		}})
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
