// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"

	"github.com/88250/lute/ast"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/workspace"
)

// UIFlashcardState belongs to one editor ticket. Source swaps during journal
// commits do not reset its round or share it with another editor.
type UIFlashcardState struct {
	mu     sync.Mutex
	rounds map[string]*flashcard.Round
}

func (s *UIFlashcardState) withRound(deck string, fn func(*flashcard.Round) (any, *flashcard.Round, error)) (any, error) {
	if s == nil {
		return nil, ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rounds == nil {
		s.rounds = map[string]*flashcard.Round{}
	}
	if s.rounds[deck] == nil && len(s.rounds) >= 32 {
		return nil, ErrArgument
	}
	out, next, err := fn(flashcard.CloneRound(s.rounds[deck]))
	if err == nil {
		if len(next.Reviewed) == 0 && len(next.Skipped) == 0 {
			delete(s.rounds, deck)
		} else {
			s.rounds[deck] = next
		}
	}
	return out, err
}

func runUIFlashcardReview(ctx context.Context, m *workspace.WorkspaceManager, ui UIRequest, state *UIFlashcardState) (any, error) {
	var input struct {
		DeckID, CardID string
		Rating         int
		ReviewedCards  []json.RawMessage
	}
	skip := ui.Endpoint == "/api/riff/skipReviewRiffCard"
	if err := Decode(bytes.NewReader(ui.Data), &input); err != nil || !nativeID.MatchString(input.DeckID) ||
		!nativeID.MatchString(input.CardID) || len(input.ReviewedCards) > 2000 ||
		(skip && input.Rating != 0 && input.Rating != -3) || (!skip && (input.Rating < int(riff.Again) || input.Rating > int(riff.Easy))) {
		return nil, ErrArgument
	}
	reviewed, err := uiReviewedCards(input.ReviewedCards)
	if err != nil {
		return nil, err
	}
	return state.withRound(input.DeckID, func(round *flashcard.Round) (any, *flashcard.Round, error) {
		if len(round.Reviewed)+len(round.Skipped) >= 2000 {
			return nil, nil, ErrArgument
		}
		request := ContentRequest{ProtocolVersion: 1, StoreID: ui.StoreID, PageID: ui.PageID,
			Action: "flashcard.review", OperationID: "ui-review-" + ast.NewNodeID()}
		plan := func(p *lifecycleEdit) error {
			l, e := p.lease()
			if e != nil {
				return e
			}
			defer l.Release()
			cfg, e := l.Configuration(p.ctx)
			if e != nil {
				return e
			}
			if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
				return ErrUnauthorized
			}
			if skip {
				return l.SkipNativeFlashcardInRound(p.ctx, input.DeckID, input.CardID, round)
			}
			q := flashcard.ReviewRequest{DeckID: input.DeckID, CardID: input.CardID, Rating: riff.Rating(input.Rating), Reviewed: reviewed}
			planned, next, e := l.PlanNativeFlashcardRoundReview(p.ctx, q, round)
			if e != nil {
				return e
			}
			round = next
			return p.stageUIFlashcardChanges(planned.Changes)
		}
		if skip {
			err := m.WithReadContext(ctx, ui.StoreID, func(c context.Context) error {
				return plan(&lifecycleEdit{m: m, ctx: c, request: request})
			})
			return nil, round, err
		}
		result, err := planContentLifecycle(ctx, m, request, plan)
		if err != nil && result.Receipt != nil && result.Receipt.SourceCommitted {
			// Source CAS succeeded. Keep the matching round even if index repair failed.
			state.rounds[input.DeckID] = round
		}
		return nil, round, err
	})
}

func uiReviewedCards(raws []json.RawMessage) ([]string, error) {
	reviewed := make([]string, 0, len(raws))
	for _, raw := range raws {
		var card struct{ CardID string }
		if json.Unmarshal(raw, &card) != nil || !nativeID.MatchString(card.CardID) {
			return nil, ErrArgument
		}
		reviewed = append(reviewed, card.CardID)
	}
	return reviewed, nil
}
