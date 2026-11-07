// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"html"
	"io/fs"
	"strings"
	"time"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	av "wiki-kernel/internal/avmodel"
	"wiki-kernel/internal/avrender"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/wikistore"
)

type NativeTemplateRequest struct {
	Action, Source, Path, ID, Name string // go, icon, render, save
	Preview, Overwrite             bool
	Entry                          *wikistore.EntryDocument
}
type NativeTemplatePlan struct {
	Text, DOM      string
	Tree           *parse.Tree
	Code           int
	SourceWrites   map[string][]byte
	SourceHashes   map[string][32]byte
	AttributeViews map[string]*av.AttributeView
}

// Templates use the same text/body/export algorithms as the compatibility
// facade. Save and AV copies are candidates; R04 owns source publication.
func (l *Lease) PlanNativeTemplate(ctx context.Context, q NativeTemplateRequest) (ret NativeTemplatePlan, err error) {
	if q.Entry != nil && (q.Action != "render" || q.ID != q.Entry.DocID) {
		return NativeTemplatePlan{}, ErrIdentity
	}
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		base, hashes, guard, e := r.documentLifecycleServices(ctx, w, bundle, "")
		if e != nil {
			return e
		}
		parser, e := base.Parser(ctx)
		if e != nil {
			return e
		}
		ret = NativeTemplatePlan{SourceWrites: map[string][]byte{}, SourceHashes: hashes, AttributeViews: map[string]*av.AttributeView{}}
		load := func(c context.Context, id string) (*parse.Tree, error) {
			lookup := id
			if q.Entry != nil {
				lookup = q.Entry.DocID
			}
			loc, ok, e := base.Lookup(c, lookup)
			if e != nil {
				return nil, e
			}
			if !ok || q.Entry != nil && (loc.Box != q.Entry.BoxID || !loc.Document) {
				return nil, ErrDocument
			}
			tree, e := base.Load(c, loc.Box, loc.Path)
			if e == nil && q.Entry != nil && (tree == nil || tree.ID != q.Entry.DocID || docview.GetNodeInTree(tree, id) == nil) {
				return nil, ErrIdentity
			}
			return tree, e
		}
		readAV := func(c context.Context, id string) (*av.AttributeView, error) {
			if q.Entry != nil {
				return nil, ErrIdentity
			}
			raw, _, e := readNativeAVSource(c, w, id)
			if e != nil {
				return nil, e
			}
			hashes["data/storage/av/"+id+".json"] = sha256.Sum256(raw)
			return decodeNativeAV(raw, id, time.Now())
		}
		main, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		readBlock := r.nativeBlock
		if q.Entry != nil {
			if r.configuration.Search == nil {
				return indexdb.ErrProjection
			}
			tree, e := load(ctx, q.Entry.DocID)
			if e != nil {
				return e
			}
			projection, e := entryReadProjection(ctx, tree, parser, r.configuration.Search.CaseSensitive)
			if e != nil {
				return e
			}
			defer projection.Close()
			main, e = projection.Database(indexdb.Main)
			if e != nil {
				return e
			}
			blocks := indexdb.CachedBlocks{DB: main, Cache: &indexdb.BlockCache{}}
			readBlock = blocks.Get
		}
		funcs := avrender.BuiltInTemplateFuncs(time.Now, parser, indexdb.TemplateFunctions(ctx, main))
		funcs["getHPathByID"] = func(id string) (string, error) {
			if q.Entry != nil {
				t, e := load(ctx, id)
				if e != nil {
					return "", e
				}
				return t.HPath, nil
			}
			loc, ok, e := base.Lookup(ctx, id)
			if e != nil {
				return "", e
			}
			if !ok {
				return "", ErrDocument
			}
			return loc.HPath, nil
		}
		funcs["statBlock"] = func(id string) (*docquery.StatResult, error) {
			return docquery.Statistics(ctx, docquery.StatRequest{Mode: "block", IDs: []string{id}}, docquery.StatServices{
				Trees: func(c context.Context, ids []string) (map[string]*parse.Tree, error) {
					out := map[string]*parse.Tree{}
					for _, id := range ids {
						t, e := load(c, id)
						if e != nil {
							return nil, e
						}
						out[id] = t
					}
					return out, c.Err()
				}, Parser: base.Parser, AttributeView: readAV,
			})
		}
		if q.Action == "go" {
			ret.Text, e = doclife.RenderCreationText(ctx, q.Source, false, funcs, nil)
			if e != nil {
				return e
			}
			return guard.Confirm(ctx)
		}
		t, e := load(ctx, q.ID)
		if e != nil {
			return e
		}
		n := docview.GetNodeInTree(t, q.ID)
		if n == nil {
			return ErrDocument
		}
		if q.Action == "save" {
			p := "templates/" + doclife.TemplateFilename(q.Name)
			raw, e := readNativeFile(ctx, w, p, 16<<20)
			if e == nil {
				hashes["data/"+p] = sha256.Sum256(raw)
				if !q.Overwrite {
					ret.Code = 1
					return guard.Confirm(ctx)
				}
			} else if !errors.Is(e, fs.ErrNotExist) {
				return e
			}
			md, e := doclife.TemplateMarkdown(ctx, t, q.ID, parser)
			if e != nil {
				return e
			}
			ret.SourceWrites["data/"+p] = md
			return guard.Confirm(ctx)
		}
		title := html.UnescapeString(n.IALAttr("name"))
		if n.Type == ast.NodeDocument {
			title = n.IALAttr("title")
		}
		data := map[string]string{"id": q.ID, "title": title, "name": html.UnescapeString(n.IALAttr("name")), "alias": html.UnescapeString(n.IALAttr("alias"))}
		source := q.Source
		if q.Action == "render" {
			p := strings.TrimPrefix(q.Path, "/")
			if !fs.ValidPath(p) || strings.Contains(p, "\\") {
				return ErrDocument
			}
			raw, e := readNativeFile(ctx, w, p, 16<<20)
			if e != nil {
				return e
			}
			hashes["data/"+p] = sha256.Sum256(raw)
			source = string(raw)
		} else if q.Action != "icon" {
			return ErrDocument
		}
		ret.Text, e = doclife.RenderCreationText(ctx, source, true, funcs, data)
		if e != nil {
			return e
		}
		if q.Action == "icon" {
			return guard.Confirm(ctx)
		}
		body := doclife.TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now, RefText: func(c context.Context, id string) (string, error) {
			return indexdb.ReferenceText(c, id, parser, readBlock)
		}}
		if q.Preview {
			body.PreviewAV = func(c context.Context, id, view string) ([]string, error) {
				a, e := readAV(c, id)
				if e != nil {
					return nil, e
				}
				v, e := a.GetCurrentView(view)
				if e != nil {
					return nil, e
				}
				return avrender.TableColumnNames(a, v)
			}
		} else {
			body.CloneAV = func(c context.Context, id string) (string, error) {
				a, e := readAV(c, id)
				if e != nil {
					return "", e
				}
				copy := a.Clone()
				if copy == nil {
					return "", ErrDocument
				}
				ret.AttributeViews[copy.ID] = copy
				return copy.ID, c.Err()
			}
		}
		ret.Tree, ret.DOM, e = doclife.RenderCreationBody(ctx, ret.Text, parser, body)
		if e != nil {
			return e
		}
		return guard.Confirm(ctx)
	})
	if err != nil {
		return NativeTemplatePlan{}, err
	}
	return
}

// Build the Single view's SQL database from the entry source, without copying
// page-wide indexes, attachment OCR, or database results into the Single view.
func entryReadProjection(ctx context.Context, tree *parse.Tree, parser *lute.Lute, caseSensitive bool) (*indexdb.Bundle, error) {
	records, err := indexdb.ProjectTree(ctx, tree, parser, indexdb.TreeServices{
		Content:          entryProjectionContent(),
		AssetHash:        func(c context.Context, _, _, _ string) (string, error) { return "", c.Err() },
		PrepareOCR:       func(c context.Context, _ *ast.Node) error { return c.Err() },
		LocalDefinitions: true,
	}, func(c context.Context, _ string) (indexdb.BlockLocation, bool, error) {
		return indexdb.BlockLocation{}, false, c.Err()
	})
	if err != nil {
		return nil, err
	}
	bundle, err := indexdb.OpenMemory(ctx)
	if err != nil {
		return nil, err
	}
	if err = bundle.Transaction(ctx, indexdb.Main, func(tx *sql.Tx) error {
		_, e := indexdb.WriteTree(ctx, tx, records, false, caseSensitive)
		return e
	}); err != nil {
		return nil, errors.Join(err, bundle.Close())
	}
	return bundle, nil
}

func entryProjectionContent() indexdb.ContentServices {
	return indexdb.ContentServices{
		AssetText:     func(c context.Context, _ string) (string, error) { return "", c.Err() },
		AttributeView: func(c context.Context, _ string, _ bool) (string, error) { return "", c.Err() },
	}
}
