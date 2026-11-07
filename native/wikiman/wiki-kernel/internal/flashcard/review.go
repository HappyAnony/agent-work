// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package flashcard

import (
	"context"
	"io/fs"
	"slices"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
)

type ReviewRequest struct {
	DeckID, CardID string
	Rating         riff.Rating
	Reviewed       []string
}
type ReviewPlan struct {
	Deck       *riff.Deck
	Round      *Round
	Log        *riff.Log
	Unreviewed int
}

func CloneRound(r *Round) *Round {
	out := NewRound()
	if r != nil {
		out.ExpectedCards = r.ExpectedCards
		for id, c := range r.Reviewed {
			out.Reviewed[id] = c.Clone()
		}
		for id, c := range r.Skipped {
			out.Skipped[id] = c.Clone()
		}
	}
	return out
}

// Re-rating after the original UI's Previous action starts from the first
// pre-review state. Clone that state again: FSRS must not mutate the undo cache.
// Round state is only accepted by the caller after its persistence succeeds.
func PrepareReview(ctx context.Context, q ReviewRequest, deck *riff.Deck, round *Round, c conf.Flashcard, exists func(context.Context, []string) (map[string]bool, error)) (ReviewPlan, error) {
	if ctx == nil || exists == nil {
		return ReviewPlan{}, ErrDependency
	}
	if !validID(q.DeckID) || !validID(q.CardID) || q.Rating < riff.Again || q.Rating > riff.Easy {
		return ReviewPlan{}, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return ReviewPlan{}, e
	}
	if deck == nil || deck.ID != q.DeckID {
		return ReviewPlan{}, fs.ErrNotExist
	}
	card := deck.GetCard(q.CardID)
	if card == nil {
		return ReviewPlan{}, fs.ErrNotExist
	}
	next := CloneRound(round)
	if original := next.Reviewed[q.CardID]; original != nil {
		deck.SetCard(original.Clone())
		delete(next.Skipped, q.CardID)
	} else {
		next.Reviewed[q.CardID] = card.Clone()
	}
	log := deck.Review(q.CardID, q.Rating)
	if log == nil {
		return ReviewPlan{}, ErrInvalid
	}
	ids := deck.GetBlockIDs()
	present, e := exists(ctx, slices.Clone(ids))
	if e != nil {
		return ReviewPlan{}, e
	}
	if e = ctx.Err(); e != nil {
		return ReviewPlan{}, e
	}
	_, unreviewed, _, _ := next.Due(deck, slices.Clone(q.Reviewed), nil, c.NewCardLimit, c.ReviewCardLimit, c.ReviewMode, func(ids []string) map[string]bool {
		ret := map[string]bool{}
		for _, id := range ids {
			ret[id] = present[id]
		}
		return ret
	})
	if unreviewed < 1 {
		next = NewRound()
	}
	if e = ctx.Err(); e != nil {
		return ReviewPlan{}, e
	}
	return ReviewPlan{Deck: deck, Round: next, Log: log, Unreviewed: unreviewed}, nil
}

type ReviewServices struct {
	Load     func(context.Context, string) (*riff.Deck, error)
	Round    func(context.Context, string) (*Round, error)
	Exists   func(context.Context, []string) (map[string]bool, error)
	Save     func(context.Context, ReviewPlan) error
	Accept   func(context.Context, string, *Round) error
	Settings conf.Flashcard
}

func ExecuteReview(ctx context.Context, q ReviewRequest, s ReviewServices) error {
	if ctx == nil || s.Load == nil || s.Round == nil || s.Exists == nil || s.Save == nil || s.Accept == nil {
		return ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	d, e := s.Load(ctx, q.DeckID)
	if e != nil {
		return e
	}
	r, e := s.Round(ctx, q.DeckID)
	if e != nil {
		return e
	}
	p, e := PrepareReview(ctx, q, d, r, s.Settings, s.Exists)
	if e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = s.Save(ctx, p); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return s.Accept(ctx, q.DeckID, CloneRound(p.Round))
}
