// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"context"
	"database/sql"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"path"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/markup"
)

type ReferenceSearchRequest struct {
	ID, RootID, Keyword      string
	BeforeLen                int
	SquareBrackets, Database bool
}
type ReferenceSearchResult struct {
	Blocks []*Block
	NewDoc bool
}
type ReferenceSearchServices struct {
	DB        *sql.DB
	Search    *conf.Search
	Editor    *conf.Editor
	Parser    *lute.Lute
	Content   indexdb.ContentServices
	Load      func(context.Context, string) (*parse.Tree, error)
	Ignore    func(context.Context) ([]string, error)
	Notebooks func(context.Context) ([]conf.Notebook, error)
	Location  func(context.Context, string) (*indexdb.BlockTreeRecord, error)
	HasPath   func(context.Context, string, string) (bool, error)
}

// SearchReferences preserves original recent-first, reference text, candidate
// filtering, notebook paths and new-document eligibility, with explicit resources.
func SearchReferences(ctx context.Context, q ReferenceSearchRequest, s ReferenceSearchServices) (out ReferenceSearchResult, err error) {
	if ctx == nil || s.DB == nil || s.Search == nil || s.Editor == nil || s.Parser == nil || s.Load == nil || s.Ignore == nil || s.Notebooks == nil || s.Location == nil || s.HasPath == nil {
		return out, errors.New("reference_search_dependencies_unbound")
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	out.Blocks = []*Block{}
	ignore, err := s.Ignore(ctx)
	if err != nil {
		return ReferenceSearchResult{}, err
	}
	onlyDoc := q.SquareBrackets && s.Editor.OnlySearchForDoc
	rows := []indexdb.BlockRecord{}
	refs := indexdb.References{DB: s.DB}
	loaded := map[string]*parse.Tree{}
	load := func(id string) (*parse.Tree, error) {
		if t, ok := loaded[id]; ok {
			return t, ctx.Err()
		}
		t, e := s.Load(ctx, id)
		if e != nil {
			return nil, e
		}
		loaded[id] = t
		return t, ctx.Err()
	}
	if q.Keyword == "" {
		recent, e := refs.Recent(ctx, onlyDoc, s.Search.TypeFilter(), ignore)
		if e != nil {
			return ReferenceSearchResult{}, e
		}
		for _, v := range recent {
			t, e := load(v.DefBlockID)
			if e != nil {
				return ReferenceSearchResult{}, e
			}
			if t == nil {
				continue
			}
			n := docview.GetNodeInTree(t, v.DefBlockID)
			if n == nil {
				continue
			}
			b, e := indexdb.ProjectBlock(ctx, t, n, s.Parser, s.Content)
			if e != nil {
				return ReferenceSearchResult{}, e
			}
			if b != nil {
				rows = append(rows, *b)
			}
		}
	} else {
		rows, err = indexdb.SearchReferenceBlocks(ctx, s.DB, *s.Search, q.Keyword, onlyDoc, ignore)
		if err != nil {
			return ReferenceSearchResult{}, err
		}
	}
	ids := []string{}
	for i := range rows {
		if err = ctx.Err(); err != nil {
			return ReferenceSearchResult{}, err
		}
		v := &rows[i]
		t, e := load(v.ID)
		if e != nil {
			return ReferenceSearchResult{}, e
		}
		if t == nil {
			continue
		}
		n := docview.GetNodeInTree(t, v.ID)
		if n == nil {
			continue
		}
		b := FromRow(v, "", q.BeforeLen, s.Search.CaseSensitive)
		if q.Keyword != "" && b.Type != "NodeAttributeView" {
			selfChild := false
			if b.IsContainerBlock() && !b.IsDoc() {
				leaf := anchor.FirstLeaf(n)
				selfChild = leaf != nil && leaf.ID == q.ID
			}
			if b.ID == q.ID || b.ID == q.RootID || selfChild {
				continue
			}
		}
		b.RefText, e = anchor.Text(ctx, n, s.Editor.BlockRefDynamicAnchorTextMaxLen, s.Content)
		if e != nil {
			return ReferenceSearchResult{}, e
		}
		out.Blocks = append(out.Blocks, b)
		ids = append(ids, b.ID)
	}
	counts, err := refs.DefinitionCounts(ctx, ids)
	if err != nil {
		return ReferenceSearchResult{}, err
	}
	books, err := s.Notebooks(ctx)
	if err != nil {
		return ReferenceSearchResult{}, err
	}
	names := map[string]string{}
	for _, b := range books {
		names[b.ID] = markup.EscapeHTML(b.Name)
	}
	for _, b := range out.Blocks {
		b.RefCount = counts[b.ID]
		b.HPath = names[b.Box] + b.HPath
		if b.IsDoc() {
			b.HPath = strings.TrimSuffix(b.HPath, path.Base(b.HPath))
		}
	}
	if q.Keyword != "" {
		if q.Database {
			out.NewDoc = true
		} else {
			loc, e := s.Location(ctx, q.ID)
			if e != nil {
				return ReferenceSearchResult{}, e
			}
			if loc != nil {
				exists, e := s.HasPath(ctx, loc.BoxID, path.Join(loc.HPath, q.Keyword))
				if e != nil {
					return ReferenceSearchResult{}, e
				}
				out.NewDoc = !exists
			}
		}
	}
	return out, ctx.Err()
}
