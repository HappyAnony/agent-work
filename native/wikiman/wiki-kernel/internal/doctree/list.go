// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package doctree

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"path"
	"sort"
	"strings"
	"wiki-kernel/internal/configtypes"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/textsort"
)

type Request struct {
	Box, Path             string
	Sort, Max             int
	Flashcard, ShowHidden bool
}
type Services struct {
	Legacy     bool
	Settings   func(context.Context, string) (int, int, error)
	List       func(context.Context, string) ([]Entry, error)
	Stat       func(context.Context, string) (*Entry, error)
	IAL        func(context.Context, string) (map[string]string, error)
	Children   func(context.Context, string) ([]Entry, error)
	IsDir      func(context.Context, string) (bool, error)
	Project    func(context.Context, Entry, map[string]string) (*File, error)
	References func(context.Context) (map[string]int, error)
	Sorts      func(context.Context, []*File) error
	Cards      *docquery.Flashcards
}

func List(ctx context.Context, q Request, s Services) (ret []*File, total int, err error) {
	if ctx == nil || s.Settings == nil || s.List == nil || s.Stat == nil || s.IAL == nil || s.Children == nil || s.IsDir == nil || s.Project == nil || s.References == nil || s.Sorts == nil {
		return nil, 0, errors.New("document_tree_dependencies_unbound")
	}
	if q.Max < 0 {
		return nil, 0, errors.New("invalid_document_tree_limit")
	}
	if q.Flashcard && (s.Cards == nil || s.Cards.Open == nil || s.Cards.Count == nil) {
		return nil, 0, errors.New("document_tree_flashcards_unbound")
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
	if err = ctx.Err(); err != nil {
		return nil, 0, err
	}
	ret = []*File{}
	if q.Flashcard {
		cards := *s.Cards
		s.Cards = &cards
		var available bool
		if err = guard(func() (e error) { available, e = s.Cards.Open(ctx); return }); err != nil {
			return nil, 0, err
		}
		if !available {
			return ret, 0, nil
		}
	}
	var boxSort, globalSort int
	if err = guard(func() (e error) { boxSort, globalSort, e = s.Settings(ctx, q.Box); return }); err != nil {
		return nil, 0, err
	}
	if q.Sort == configtypes.SortModeUnassigned {
		q.Sort = globalSort
		if boxSort != configtypes.SortModeFileTree {
			q.Sort = boxSort
		}
	}
	var entries []Entry
	if err = guard(func() (e error) { entries, e = s.List(ctx, q.Path); return }); err != nil {
		return nil, 0, err
	}
	docs := []*File{}
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return nil, 0, err
		}
		file := entry
		folder := ""
		if entry.Directory {
			if !ast.IsNodeIDPattern(entry.Name) {
				continue
			}
			parent := strings.TrimSuffix(entry.Path, "/") + ".docj"
			var found *Entry
			if err = guard(func() (e error) { found, e = s.Stat(ctx, parent); return }); err != nil {
				return nil, 0, err
			}
			if found == nil {
				continue
			}
			file = *found
			folder = entry.Path
		} else {
			if strings.HasSuffix(entry.Name, ".docj") && !ast.IsNodeIDPattern(strings.TrimSuffix(entry.Name, ".docj")) {
				continue
			}
			var exists bool
			if err = guard(func() (e error) { exists, e = s.IsDir(ctx, strings.TrimSuffix(entry.Path, ".docj")); return }); err != nil {
				return nil, 0, err
			}
			if exists {
				continue
			}
		}
		var ial map[string]string
		if err = guard(func() (e error) { ial, e = s.IAL(ctx, file.Path); return }); err != nil {
			return nil, 0, err
		}
		if ial == nil || (!q.ShowHidden && ial["custom-hidden"] == "true") {
			continue
		}
		var doc *File
		if err = guard(func() (e error) { doc, e = s.Project(ctx, file, ial); return }); err != nil {
			return nil, 0, err
		}
		if doc == nil {
			return nil, 0, errors.New("invalid_document_tree_file")
		}
		copy := *doc
		doc = &copy // Count/sort/child projection must not edit an owner cache.
		if folder != "" {
			var children []Entry
			e := guard(func() (e error) { children, e = s.Children(ctx, folder); return })
			if e != nil {
				if !s.Legacy || ctx.Err() != nil {
					return nil, 0, e
				}
			} else {
				for _, child := range children {
					var attrs map[string]string
					if err = guard(func() (e error) { attrs, e = s.IAL(ctx, path.Join(folder, child.Name)); return }); err != nil {
						return nil, 0, err
					}
					if attrs["custom-hidden"] == "true" {
						continue
					}
					if strings.HasSuffix(child.Name, ".docj") {
						doc.SubFileCount++
					}
				}
			}
		}
		if q.Flashcard {
			var c docquery.Counts
			if err = guard(func() (e error) {
				c, e = s.Cards.Count(ctx, "root", strings.TrimSuffix(path.Base(file.Path), ".docj"))
				return
			}); err != nil {
				return nil, 0, err
			}
			if c.Total < 1 {
				continue
			}
			doc.NewFlashcardCount, doc.DueFlashcardCount, doc.FlashcardCount = c.New, c.Due, c.Total
		}
		docs = append(docs, doc)
	}
	var refs map[string]int
	if err = guard(func() (e error) { refs, e = s.References(ctx); return }); err != nil {
		return nil, 0, err
	}
	for _, doc := range docs {
		if count := refs[doc.ID]; count > 0 {
			doc.Count = count
		}
	}
	if q.Sort == configtypes.SortModeCustom {
		if err = guard(func() error { return s.Sorts(ctx, docs) }); err != nil {
			return nil, 0, err
		}
	}
	if q.Sort >= configtypes.SortModeNameASC && q.Sort <= configtypes.SortModeSubDocCountDESC {
		sort.Slice(docs, func(i, j int) bool {
			a, b := docs[i], docs[j]
			switch q.Sort {
			case configtypes.SortModeNameASC:
				return textsort.FileTreeCompare(a.Name, b.Name)
			case configtypes.SortModeNameDESC:
				return textsort.FileTreeCompare(b.Name, a.Name)
			case configtypes.SortModeAlphanumASC:
				return textsort.NaturalCompare(a.Name, b.Name)
			case configtypes.SortModeAlphanumDESC:
				return textsort.NaturalCompare(b.Name, a.Name)
			case configtypes.SortModeUpdatedASC:
				return a.Mtime < b.Mtime
			case configtypes.SortModeUpdatedDESC:
				return a.Mtime > b.Mtime
			case configtypes.SortModeCustom:
				if a.Sort == b.Sort {
					return idPrefix(a.ID) > idPrefix(b.ID)
				}
				return a.Sort < b.Sort
			case configtypes.SortModeRefCountASC:
				return a.Count < b.Count
			case configtypes.SortModeRefCountDESC:
				return a.Count > b.Count
			case configtypes.SortModeCreatedASC:
				return a.CTime < b.CTime
			case configtypes.SortModeCreatedDESC:
				return a.CTime > b.CTime
			case configtypes.SortModeSizeASC:
				return a.Size < b.Size
			case configtypes.SortModeSizeDESC:
				return a.Size > b.Size
			case configtypes.SortModeSubDocCountASC:
				return a.SubFileCount < b.SubFileCount
			case configtypes.SortModeSubDocCountDESC:
				return a.SubFileCount > b.SubFileCount
			}
			return false
		})
	}
	if err = ctx.Err(); err != nil {
		return nil, 0, err
	}
	total = len(docs)
	if q.Max < len(docs) {
		docs = docs[:q.Max]
	}
	return docs, total, nil
}
func idPrefix(id string) string { return id[:min(14, len(id))] }
