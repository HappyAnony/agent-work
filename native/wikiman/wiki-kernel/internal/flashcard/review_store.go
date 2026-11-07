package flashcard

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io/fs"
	"sort"
)

var ErrReviewConflict = errors.New("flashcard_review_conflict")

type PreparedReview struct {
	Token      string
	Changes    []Change
	Unreviewed int
}
type pendingReview struct {
	token    string
	version  uint64
	round    *Round
	expected memory
}

// Preparation never changes the live round. Confirmation accepts only a token
// from this Store, current round version and exact persisted afterimages. R04/R05
// supply the source commit; this is its read-only confirmation seam, not a writer.
func (s *Store) PrepareReview(ctx context.Context, q ReviewRequest, exists func(context.Context, []string) (map[string]bool, error)) (out PreparedReview, err error) {
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		candidate, next, working, e := s.planReview(ctx, q, s.rounds[q.DeckID], exists)
		if e != nil {
			return e
		}
		out = candidate
		out.Token = rand.Text()
		budget := 0
		count := 1
		for id, pending := range s.pendingReviews {
			if id == q.DeckID {
				continue
			}
			count++
			for _, b := range pending.expected {
				budget += len(b)
			}
		}
		for _, b := range working {
			budget += len(b)
		}
		if count > 8 || budget > 64<<20 {
			return ErrInvalid
		}
		s.pendingReviews[q.DeckID] = pendingReview{token: out.Token, version: s.roundVersions[q.DeckID], round: next, expected: clone(working)}
		return nil
	})
	if err != nil {
		return PreparedReview{}, err
	}
	return
}

// PlanReview returns a candidate and detached next round for a caller that owns
// both the source transaction and the review UI's lifetime.
func (s *Store) PlanReview(ctx context.Context, q ReviewRequest, round *Round, exists func(context.Context, []string) (map[string]bool, error)) (out PreparedReview, next *Round, err error) {
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		cards := q.DeckID + ".cards"
		if round != nil && round.ExpectedCards != ([32]byte{}) && round.ExpectedCards != sha256.Sum256(s.files[cards]) {
			round = nil
		}
		candidate, planned, working, e := s.planReview(ctx, q, round, exists)
		if e != nil {
			return e
		}
		planned.ExpectedCards = sha256.Sum256(working[cards])
		out, next = candidate, planned
		return nil
	})
	return
}

func (s *Store) planReview(ctx context.Context, q ReviewRequest, round *Round, exists func(context.Context, []string) (map[string]bool, error)) (PreparedReview, *Round, memory, error) {
	if s.decks[q.DeckID] == nil {
		return PreparedReview{}, nil, nil, fs.ErrNotExist
	}
	before := memory{}
	for _, p := range []string{q.DeckID + ".deck", q.DeckID + ".cards"} {
		if b, ok := s.files[p]; ok {
			before[p] = bytes.Clone(b)
		}
	}
	logPath := "logs/" + s.clock().Format("200601") + ".msgpack"
	b, e := s.source.Read(ctx, logPath)
	if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return PreparedReview{}, nil, nil, e
	}
	if e == nil {
		before[logPath] = bytes.Clone(b)
	}
	working := clone(before)
	d, e := s.decode(working, q.DeckID)
	if e != nil {
		return PreparedReview{}, nil, nil, e
	}
	p, e := PrepareReview(ctx, q, d, round, s.settings, exists)
	if e != nil {
		return PreparedReview{}, nil, nil, e
	}
	if e = d.Save(); e != nil {
		return PreparedReview{}, nil, nil, e
	}
	if e = d.SaveLog(p.Log); e != nil {
		return PreparedReview{}, nil, nil, e
	}
	out := PreparedReview{Unreviewed: p.Unreviewed}
	keys := []string{}
	for name := range working {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		old, ok := before[name]
		if !ok || !bytes.Equal(old, working[name]) {
			out.Changes = append(out.Changes, Change{Path: name, Before: bytes.Clone(old), After: bytes.Clone(working[name]), Existed: ok})
		}
	}
	if e = ctx.Err(); e != nil {
		return PreparedReview{}, nil, nil, e
	}
	return out, p.Round, working, nil
}

func (s *Store) ConfirmReview(ctx context.Context, token string) error {
	return s.run(ctx, func() error {
		var deck string
		var p pendingReview
		for id, candidate := range s.pendingReviews {
			if candidate.token == token {
				deck = id
				p = candidate
				break
			}
		}
		if deck == "" || p.version != s.roundVersions[deck] {
			return ErrReviewConflict
		}
		for name, want := range p.expected {
			got, e := s.source.Read(ctx, name)
			if e != nil {
				return e
			}
			if !bytes.Equal(got, want) {
				return ErrReviewConflict
			}
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		s.rounds[deck] = CloneRound(p.round)
		for _, name := range []string{deck + ".deck", deck + ".cards"} {
			s.files[name] = bytes.Clone(p.expected[name])
		}
		s.roundVersions[deck]++
		delete(s.pendingReviews, deck)
		return nil
	})
}
func (s *Store) AbortReview(ctx context.Context, token string) error {
	return s.run(ctx, func() error {
		for id, p := range s.pendingReviews {
			if p.token == token {
				delete(s.pendingReviews, id)
				return nil
			}
		}
		return ErrReviewConflict
	})
}
