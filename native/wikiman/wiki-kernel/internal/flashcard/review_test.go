package flashcard

import (
	"context"
	"errors"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
)

func allExisting(_ context.Context, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}
func TestFlashcardReviewRepeatedUndoAndFailureDoNotRewriteOriginalCache(t *testing.T) {
	ctx := context.Background()
	settings := *conf.NewFlashcard()
	now := time.Now().Add(-time.Hour)
	clock := func() time.Time { return now }
	deck, e := NewDeck(BuiltinDeckID, "review", settings, clock)
	if e != nil {
		t.Fatal(e)
	}
	deck.AddCard(cardID, blockID)
	deck.AddCard("20261028000000-card002", mutationBlock2)
	round := NewRound()
	saved, accepted := 0, 0
	failSave := false
	s := ReviewServices{Load: func(context.Context, string) (*riff.Deck, error) { return CloneDeck(deck, settings, clock) }, Round: func(context.Context, string) (*Round, error) { return round, nil }, Settings: settings, Exists: allExisting,
		Save: func(_ context.Context, p ReviewPlan) error {
			if failSave {
				return errors.New("disk full")
			}
			saved++
			deck = p.Deck
			return nil
		}, Accept: func(_ context.Context, _ string, r *Round) error { accepted++; round = r; return nil }}
	q := ReviewRequest{DeckID: BuiltinDeckID, CardID: cardID, Rating: riff.Good, Reviewed: []string{cardID}}
	for _, rating := range []riff.Rating{riff.Good, riff.Again, riff.Easy, riff.Hard} {
		q.Rating = rating
		if e = ExecuteReview(ctx, q, s); e != nil {
			t.Fatal(e)
		}
		if deck.GetCard(cardID).GetReps() != 1 || round.Reviewed[cardID] == nil || round.Reviewed[cardID].GetReps() != 0 {
			t.Fatal("undo baseline mutated", deck.GetCard(cardID), round)
		}
	}
	if saved != 4 || accepted != 4 {
		t.Fatal(saved, accepted)
	}
	failSave = true
	if e = ExecuteReview(ctx, q, s); e == nil || accepted != 4 || round.Reviewed[cardID].GetReps() != 0 {
		t.Fatal("failed save advanced round", e, accepted)
	}
	cancelled, cancel := context.WithCancel(ctx)
	s.Exists = func(ctx context.Context, ids []string) (map[string]bool, error) {
		cancel()
		return allExisting(ctx, ids)
	}
	if e = ExecuteReview(cancelled, q, s); !errors.Is(e, context.Canceled) || saved != 4 || accepted != 4 {
		t.Fatal(e, saved, accepted)
	}
}
func TestFlashcardReviewTokenConfirmationAndInvalidation(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Add(-time.Hour)
	clock := func() time.Time { return now }
	m := seeded(t, now)
	settings := *conf.NewFlashcard()
	d, e := riff.LoadDeck(m, BuiltinDeckID, settings.RequestRetention, settings.MaximumInterval, settings.Weights, clock)
	if e != nil {
		t.Fatal(e)
	}
	d.AddCard("20261028000000-card002", mutationBlock2)
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	a, b := Open(source(m), settings, clock), Open(source(m), settings, clock)
	defer a.Close()
	defer b.Close()
	q := ReviewRequest{DeckID: BuiltinDeckID, CardID: cardID, Rating: riff.Good, Reviewed: []string{cardID}}
	prepared, e := a.PrepareReview(ctx, q, allExisting)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.ConfirmReview(ctx, prepared.Token); e == nil {
		t.Fatal("uncommitted review confirmed")
	}
	if e = b.ConfirmReview(ctx, prepared.Token); !errors.Is(e, ErrReviewConflict) {
		t.Fatal("foreign token", e)
	}
	for _, c := range prepared.Changes {
		m[c.Path] = c.After
	}
	if e = a.ConfirmReview(ctx, prepared.Token); e != nil {
		t.Fatal(e)
	}
	if a.rounds[BuiltinDeckID].Reviewed[cardID].GetReps() != 0 {
		t.Fatal("original lost")
	}
	if e = a.ConfirmReview(ctx, prepared.Token); !errors.Is(e, ErrReviewConflict) {
		t.Fatal("replayed token", e)
	}
	q.Rating = riff.Easy
	again, e := a.PrepareReview(ctx, q, allExisting)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range again.Changes {
		m[c.Path] = c.After
	}
	if e = a.ConfirmReview(ctx, again.Token); e != nil {
		t.Fatal(e)
	}
	cards, e := a.Cards(ctx, BuiltinDeckID, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range cards {
		if c.ID() == cardID && c.GetReps() != 1 {
			t.Fatal("rerating incremented reps", c)
		}
	}
	pending, e := a.PrepareReview(ctx, q, allExisting)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.Due(ctx, BuiltinDeckID, []string{blockID, mutationBlock2}, nil); e != nil {
		t.Fatal(e)
	}
	if e = a.ConfirmReview(ctx, pending.Token); !errors.Is(e, ErrReviewConflict) {
		t.Fatal("new round accepted stale token", e)
	}
	pending, e = a.PrepareReview(ctx, q, allExisting)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.AbortReview(ctx, pending.Token); e != nil {
		t.Fatal(e)
	}
	if e = a.ConfirmReview(ctx, pending.Token); !errors.Is(e, ErrReviewConflict) {
		t.Fatal(e)
	}
}

func TestFlashcardExternalSourceChangeInvalidatesUndoAndPending(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Add(-time.Hour)
	clock := func() time.Time { return now }
	settings := *conf.NewFlashcard()
	m := seeded(t, now)
	s := Open(source(m), settings, clock)
	defer s.Close()
	q := ReviewRequest{DeckID: BuiltinDeckID, CardID: cardID, Rating: riff.Good, Reviewed: []string{cardID}}
	p, e := s.PrepareReview(ctx, q, allExisting)
	if e != nil {
		t.Fatal(e)
	}
	d, e := riff.LoadDeck(m, BuiltinDeckID, settings.RequestRetention, settings.MaximumInterval, settings.Weights, clock)
	if e != nil {
		t.Fatal(e)
	}
	d.AddCard("20261028000000-card002", mutationBlock2)
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Cards(ctx, BuiltinDeckID, nil); e != nil {
		t.Fatal(e)
	}
	if e = s.ConfirmReview(ctx, p.Token); !errors.Is(e, ErrReviewConflict) {
		t.Fatal("stale source token accepted", e)
	}
	s.rounds[BuiltinDeckID] = NewRound()
	s.rounds[BuiltinDeckID].Reviewed[cardID] = d.GetCard(cardID).Clone()
	d.Review(cardID, riff.Good)
	if e = d.Save(); e != nil {
		t.Fatal(e)
	}
	p, e = s.PrepareReview(ctx, q, allExisting)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range p.Changes {
		m[c.Path] = c.After
	}
	if e = s.ConfirmReview(ctx, p.Token); e != nil {
		t.Fatal(e)
	}
	cards, e := s.Cards(ctx, BuiltinDeckID, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range cards {
		if c.ID() == cardID && c.GetReps() != 2 {
			t.Fatal("agent update overwritten by old undo", c)
		}
	}
	_, e = s.PrepareReview(ctx, q, allExisting)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	if len(s.pendingReviews) != 0 || len(s.roundVersions) != 0 {
		t.Fatal("closed owner retained review state")
	}
}
