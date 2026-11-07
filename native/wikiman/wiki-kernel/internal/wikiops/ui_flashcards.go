// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"

	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/workspace"
)

var uiFlashcardRoutes = map[string]uiRoute{
	"/api/riff/getRiffDecks": uiRouteFor(func(readEmpty) (uiCall, error) {
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			decks, err := l.NativeFlashcardDecks(ctx)
			if err != nil {
				return nil, err
			}
			result := make([]map[string]any, 0, len(decks))
			for _, deck := range decks {
				result = append(result, uiDeck(deck))
			}
			return result, nil
		}}, nil
	}),
	"/api/riff/get":                  uiFlashcardPage("deck"),
	"/api/riff/getNotebookRiffCards": uiFlashcardPage("notebook"),
	"/api/riff/getTreeRiffCards":     uiFlashcardPage("tree"),
	"/api/riff/getRiffCardsByBlockIDs": uiRouteFor(func(q struct{ BlockIDs []string }) (uiCall, error) {
		if len(q.BlockIDs) > 1000 || slices.ContainsFunc(q.BlockIDs, func(id string) bool { return !nativeID.MatchString(id) }) {
			return uiCall{}, ErrArgument
		}
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			result, err := l.NativeFlashcardPage(ctx, flashcard.PageRequest{Scope: "blocks", IDs: q.BlockIDs})
			return map[string]any{"blocks": result.Blocks}, err
		}}, nil
	}),
}

func uiFlashcardPage(scope string) uiRoute {
	return uiRouteFor(func(q struct {
		ID             string
		Page, PageSize int
	}) (uiCall, error) {
		if !nativeID.MatchString(q.ID) {
			return uiCall{}, ErrArgument
		}
		if q.PageSize <= 0 {
			q.PageSize = 20
		}
		if q.PageSize > 1000 {
			return uiCall{}, ErrArgument
		}
		return uiCall{pageWide: true, run: func(ctx context.Context, l *workspace.Lease) (any, error) {
			return l.NativeFlashcardPage(ctx, flashcard.PageRequest{Scope: scope, ID: q.ID, Page: max(q.Page, 1), PageSize: q.PageSize})
		}}, nil
	})
}

var uiFlashcardDue = map[string]string{
	"/api/riff/getRiffDueCards":         "deck",
	"/api/riff/getNotebookRiffDueCards": "notebook",
	"/api/riff/getTreeRiffDueCards":     "tree",
}

func runUIDueCards(ctx context.Context, m *workspace.WorkspaceManager, ui UIRequest, scope string, state *UIFlashcardState) (any, error) {
	var q struct {
		DeckID, Notebook, RootID string
		ReviewedCards            []json.RawMessage
	}
	if err := Decode(bytes.NewReader(ui.Data), &q); err != nil || len(q.ReviewedCards) > 2000 {
		return nil, ErrArgument
	}
	deck, id := flashcard.BuiltinDeckID, q.RootID
	switch scope {
	case "deck":
		deck, id = q.DeckID, q.DeckID
	case "notebook":
		id = q.Notebook
	}
	if !nativeID.MatchString(id) {
		return nil, ErrArgument
	}
	reviewed, err := uiReviewedCards(q.ReviewedCards)
	if err != nil {
		return nil, err
	}
	return state.withRound(deck, func(round *flashcard.Round) (any, *flashcard.Round, error) {
		var result workspace.NativeFlashcardDueView
		err := m.WithReadContext(ctx, ui.StoreID, func(c context.Context) error {
			l, e := m.Acquire(c, ui.StoreID, ui.PageID)
			if e != nil {
				return e
			}
			defer l.Release()
			projection, e := l.PrepareNativeReads(c, false)
			if e != nil {
				return e
			}
			result, e = l.NativeFlashcardDueRound(c, deck, scope, id, reviewed, round)
			if e != nil {
				return e
			}
			return l.VerifyNativeReadSource(c, projection.SourceHash)
		})
		return result, round, err
	})
}
