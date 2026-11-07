// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package docquery

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

type Request struct {
	Keyword    string
	Flashcard  bool
	ExcludeIDs []string
}
type Counts struct{ New, Due, Total int }
type Flashcards struct {
	Open  func(context.Context) (bool, error)
	Count func(context.Context, string, string) (Counts, error)
}
type Services struct {
	Notebooks func(context.Context) ([]conf.Notebook, error)
	Roots     func(context.Context, []string, []string) ([]indexdb.BlockRecord, error)
	Cards     *Flashcards
}

func Search(ctx context.Context, request Request, s Services) ([]map[string]string, error) {
	if ctx == nil || s.Notebooks == nil || s.Roots == nil {
		return nil, errors.New("document_search_dependencies_unbound")
	}
	if request.Flashcard && (s.Cards == nil || s.Cards.Open == nil || s.Cards.Count == nil) {
		return nil, errors.New("document_search_flashcards_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request.ExcludeIDs = slices.Clone(request.ExcludeIDs)
	if s.Cards != nil {
		copy := *s.Cards
		s.Cards = &copy
	}
	ret := []map[string]string{}
	if request.Flashcard {
		available, err := s.Cards.Open(ctx)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if !available {
			return ret, nil
		}
	}
	items, err := s.Notebooks(ctx)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	boxes := map[string]conf.Notebook{}
	for _, box := range items {
		if !box.Closed {
			boxes[box.ID] = box
		}
	}
	words := strings.Fields(request.Keyword)
	appendResult := func(scope, id, path, hpath string, box conf.Notebook) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := map[string]string{"path": path, "hPath": hpath, "box": box.ID, "boxIcon": box.Icon}
		if request.Flashcard {
			counts, err := s.Cards.Count(ctx, scope, id)
			if err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			if counts.Total < 1 {
				return nil
			}
			row["newFlashcardCount"], row["dueFlashcardCount"], row["flashcardCount"] = strconv.Itoa(counts.New), strconv.Itoa(counts.Due), strconv.Itoa(counts.Total)
		}
		ret = append(ret, row)
		return nil
	}
	for _, box := range boxes {
		// Original gulu.Str.Contains compares the notebook name with each
		// complete token (any token), rather than a substring/AND match.
		if len(words) == 0 || slices.Contains(words, box.Name) {
			if err = appendResult("box", box.ID, "/", box.Name+"/", box); err != nil {
				return nil, err
			}
		}
	}
	if len(words) > 0 {
		rows, e := s.Roots(ctx, slices.Clone(words), slices.Clone(request.ExcludeIDs))
		if e != nil {
			return nil, e
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		for _, row := range rows {
			box, ok := boxes[row.Box]
			if !ok {
				continue
			}
			if err = appendResult("root", row.ID, row.Path, box.Name+row.HPath, box); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i]["hPath"] < ret[j]["hPath"] })
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}
