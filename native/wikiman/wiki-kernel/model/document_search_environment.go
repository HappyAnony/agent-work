// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/flashcard/riff"
)

type documentSearchEnvironment struct {
	ctx      context.Context
	services docquery.Services
}

func legacyNavigationFlashcards() *docquery.Flashcards {
	var deck *riff.Deck
	var ids []string
	return &docquery.Flashcards{Open: func(context.Context) (bool, error) {
		deck = Decks[builtinDeckID]
		if deck == nil {
			return false, nil
		}
		ids = deck.GetBlockIDs()
		return true, nil
	},
		Count: func(_ context.Context, scope, id string) (c docquery.Counts, err error) {
			if scope == "box" {
				c.New, c.Due, c.Total = countBoxFlashcard(id, deck, ids)
			} else {
				c.New, c.Due, c.Total = countTreeFlashcard(id, deck, ids)
			}
			return
		},
	}
}

func searchDocumentsWithEnvironment(q docquery.Request, e *documentSearchEnvironment) ([]map[string]string, error) {
	if e == nil {
		return nil, errors.New("document_search_dependencies_unbound")
	}
	return docquery.Search(e.ctx, q, e.services)
}
func (tx *Transaction) searchDocuments(q docquery.Request) ([]map[string]string, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.documentSearch == nil {
			return nil, errors.New("document_search_dependencies_unbound")
		}
		e := *tx.environment.documentSearch
		e.ctx = tx.environment.ctx
		return searchDocumentsWithEnvironment(q, &e)
	}
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	return SearchDocsByKeywordContext(ctx, q.Keyword, q.Flashcard, q.ExcludeIDs)
}
