package flashcard

import (
	"context"
	"io/fs"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
)

func (s *Store) Configure(ctx context.Context, settings conf.Flashcard) error {
	return s.run(ctx, func() error {
		if s.settings != settings {
			clear(s.pendingReviews)
			for id := range s.roundVersions {
				s.roundVersions[id]++
			}
		}
		s.settings = settings
		return nil
	})
}

type DueResult struct {
	Cards                []riff.Card
	Unreviewed, New, Old int
}

// Empty existingIDs means no blocks exist; it never means an unrestricted deck.
// Round caches are keyed by deck as well as by the owning workspace.
func (s *Store) Due(ctx context.Context, deck string, existingIDs, reviewed []string) (DueResult, error) {
	return s.DueWithLimits(ctx, deck, existingIDs, reviewed, nil)
}
func (s *Store) DueWithLimits(ctx context.Context, deck string, existingIDs, reviewed []string, limits *conf.Flashcard) (DueResult, error) {
	return s.DueWithRound(ctx, deck, existingIDs, reviewed, limits, nil)
}

func (s *Store) DueWithRound(ctx context.Context, deck string, existingIDs, reviewed []string, limits *conf.Flashcard, round *Round) (out DueResult, err error) {
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		d := s.decks[deck]
		if d == nil {
			return fs.ErrNotExist
		}
		if round == nil {
			round = s.rounds[deck]
			if round == nil {
				round = NewRound()
				s.rounds[deck] = round
			}
			if len(reviewed) == 0 {
				s.roundVersions[deck]++
				delete(s.pendingReviews, deck)
			}
		}
		exists := map[string]bool{}
		for _, id := range existingIDs {
			exists[id] = true
		}
		settings := s.settings
		if limits != nil {
			settings = *limits
		}
		cards, n, newCount, oldCount := round.Due(d, reviewed, existingIDs, settings.NewCardLimit, settings.ReviewCardLimit, settings.ReviewMode, func(ids []string) map[string]bool {
			out := map[string]bool{}
			for _, id := range ids {
				out[id] = exists[id]
			}
			return out
		})
		out = DueResult{Cards: []riff.Card{}, Unreviewed: n, New: newCount, Old: oldCount}
		for _, c := range cards {
			out.Cards = append(out.Cards, c.Clone())
		}
		return ctx.Err()
	})
	if err != nil {
		return DueResult{}, err
	}
	return
}
func (s *Store) Skip(ctx context.Context, deck, card string) error {
	return s.SkipWithRound(ctx, deck, card, nil)
}

func (s *Store) SkipWithRound(ctx context.Context, deck, card string, round *Round) error {
	return s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		d := s.decks[deck]
		if d == nil || d.GetCard(card) == nil {
			return fs.ErrNotExist
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if round == nil {
			round = s.rounds[deck]
			if round == nil {
				round = NewRound()
				s.rounds[deck] = round
			}
			s.roundVersions[deck]++
			delete(s.pendingReviews, deck)
		}
		round.Skipped[card] = d.GetCard(card).Clone()
		return nil
	})
}
