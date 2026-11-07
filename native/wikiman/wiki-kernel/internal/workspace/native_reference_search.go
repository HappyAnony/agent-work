// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/parse"
	"io/fs"
	"wiki-kernel/conf"
	"wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/wikistore"
)

func (r *resources) readBlockContent(ctx context.Context, w *WorkspaceContext) indexdb.ContentServices {
	content := r.nativeContent
	content.AttributeView = func(c context.Context, id string, full bool) (string, error) {
		raw, doc, e := readNativeAVSource(c, w, id)
		if e != nil {
			return "", e
		}
		if !full {
			return doc.Name, c.Err()
		}
		a := &avmodel.AttributeView{}
		if e := json.Unmarshal(raw, a); e != nil {
			return "", e
		}
		return avrender.SearchText(c, a)
	}
	return content
}

func (l *Lease) SearchNativeEntryReferences(ctx context.Context, entry wikistore.EntryDocument, q blockview.ReferenceSearchRequest) (out blockview.ReferenceSearchResult, err error) {
	if q.RootID != entry.DocID || q.Database {
		return out, ErrIdentity
	}
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.Search == nil {
			return indexdb.ErrProjection
		}
		tree, e := r.locatedTree(ctx, w, entry.BoxID, entry.DocID)
		if e != nil {
			return e
		}
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		projection, e := entryReadProjection(ctx, tree, parser, r.configuration.Search.CaseSensitive)
		if e != nil {
			return e
		}
		defer projection.Close()
		db, e := projection.Database(indexdb.Main)
		if e != nil {
			return e
		}
		out, e = blockview.SearchReferences(ctx, q, blockview.ReferenceSearchServices{
			DB: db, Search: r.configuration.Search, Editor: r.configuration.Editor, Parser: parser,
			Content: entryProjectionContent(),
			Load: func(c context.Context, id string) (*parse.Tree, error) {
				if docview.GetNodeInTree(tree, id) == nil {
					return nil, c.Err()
				}
				return tree, c.Err()
			},
			Ignore: func(c context.Context) ([]string, error) {
				raw, e := readNativeFile(c, w, ".wiki/refsearchignore", 1<<20)
				if e != nil && !errors.Is(e, fs.ErrNotExist) {
					return nil, e
				}
				return indexdb.SearchIgnoreLines(raw), c.Err()
			},
			Notebooks: func(c context.Context) ([]conf.Notebook, error) {
				books, e := r.configurationBusiness.Notebooks(c)
				for _, book := range books {
					if book.ID == entry.BoxID {
						return []conf.Notebook{book}, e
					}
				}
				return nil, e
			},
			// A Single ticket cannot create another document or probe its path.
			Location: func(c context.Context, _ string) (*indexdb.BlockTreeRecord, error) { return nil, c.Err() },
			HasPath:  func(c context.Context, _, _ string) (bool, error) { return false, c.Err() },
		})
		return e
	})
	return
}
func (l *Lease) SearchNativeReferences(ctx context.Context, q blockview.ReferenceSearchRequest) (out blockview.ReferenceSearchResult, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if !r.blockTreeReady {
			if e := r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		db, e := b.Database(indexdb.Main)
		if e != nil {
			return e
		}
		bt, e := b.Database(indexdb.BlockTree)
		if e != nil {
			return e
		}
		registry := indexdb.BlockTrees{DB: bt}
		parser, e := r.documentInfoParser(ctx, w)
		if e != nil {
			return e
		}
		out, e = blockview.SearchReferences(ctx, q, blockview.ReferenceSearchServices{DB: db, Search: r.configuration.Search, Editor: r.configuration.Editor, Parser: parser, Content: r.readBlockContent(ctx, w), Notebooks: r.configurationBusiness.Notebooks,
			Load: func(c context.Context, id string) (*parse.Tree, error) {
				trees, e := r.documentInfoTrees(c, w, []string{id})
				return trees[id], e
			},
			Ignore: func(c context.Context) ([]string, error) {
				raw, e := readNativeFile(c, w, ".wiki/refsearchignore", 1<<20)
				if e != nil && !errors.Is(e, fs.ErrNotExist) {
					return nil, e
				}
				return indexdb.SearchIgnoreLines(raw), c.Err()
			},
			Location: func(c context.Context, id string) (*indexdb.BlockTreeRecord, error) {
				rows, e := registry.Query(c, indexdb.BlockTreeFilter{Kind: "id", Value: id})
				if e != nil || len(rows) == 0 {
					return nil, e
				}
				return &rows[0], nil
			},
			HasPath: func(c context.Context, box, p string) (bool, error) {
				rows, e := registry.Query(c, indexdb.BlockTreeFilter{Kind: "root-hpath", BoxID: box, Value: p})
				return len(rows) > 0, e
			},
		})
		return e
	})
	if err != nil {
		return blockview.ReferenceSearchResult{}, err
	}
	return
}
