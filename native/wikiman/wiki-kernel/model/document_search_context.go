// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"slices"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexdb"
)

func (o *workspaceReadOwner) navigationCards() *docquery.Flashcards {
	var deck *riff.Deck
	return &docquery.Flashcards{
		Open: func(c context.Context) (bool, error) {
			if e := o.r.guard(c); e != nil {
				return false, e
			}
			if e := deckLock.LockContext(c); e != nil {
				return false, e
			}
			defer deckLock.Unlock()
			current := Decks[builtinDeckID]
			if current == nil {
				return false, nil
			}
			if o.settings.Flashcard == nil {
				return false, conf.ErrBusinessUnbound
			}
			var e error
			deck, e = flashcard.CloneDeck(current, *o.settings.Flashcard, time.Now)
			if e != nil {
				return false, e
			}
			return true, o.r.guard(c)
		},
		Count: func(c context.Context, scope, id string) (docquery.Counts, error) {
			if e := o.r.guard(c); e != nil {
				return docquery.Counts{}, e
			}
			f := indexdb.BlockTreeFilter{Kind: "box", Value: id}
			box := ""
			if scope != "box" {
				loc, e := o.r.referenceLookup(c, id)
				if e != nil {
					return docquery.Counts{}, e
				}
				if loc == nil {
					return docquery.Counts{}, nil
				}
				f = indexdb.BlockTreeFilter{Kind: "prefix", Value: strings.TrimSuffix(loc.Path, ".docj")}
				box = loc.BoxID
			}
			rows, e := o.r.registry.Query(c, f)
			if e != nil {
				return docquery.Counts{}, e
			}
			ids := []string{}
			for _, row := range rows {
				if box == "" || row.BoxID == box {
					ids = append(ids, row.ID)
				}
			}
			out := flashcard.Count(deck, ids)
			return out, o.r.guard(c)
		},
	}
}

func SearchDocsByKeywordContext(ctx context.Context, keyword string, cards bool, exclude []string) ([]map[string]string, error) {
	exclude = slices.Clone(exclude)
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) ([]map[string]string, error) {
		if o.settings.Search == nil {
			return nil, conf.ErrBusinessUnbound
		}
		return docquery.Search(c, docquery.Request{Keyword: keyword, Flashcard: cards, ExcludeIDs: exclude}, docquery.Services{
			Notebooks: o.base.owner.business.Notebooks,
			Roots: func(c context.Context, words, exclude []string) ([]indexdb.BlockRecord, error) {
				return indexdb.SearchDocumentRoots(c, o.r.main.DB, *o.settings.Search, words, exclude)
			},
			Cards: o.navigationCards(),
		})
	})
}
