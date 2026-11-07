// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package flashcard

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/flashcard/riff"
)

const BuiltinDeckID = "20230218211946-2kw8jgx"

var ErrClosed = errors.New("flashcard_closed")
var ErrInvalid = errors.New("flashcard_invalid")

// Source functions must address the same owner and honor cancellation. List
// returns relative deck names; Read also serves original monthly review logs.
type Source struct {
	List func(context.Context) ([]string, error)
	Read func(context.Context, string) ([]byte, error)
}
type memory map[string][]byte

func (m memory) Read(p string) ([]byte, error) {
	b, ok := m[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return bytes.Clone(b), nil
}
func (m memory) Write(p string, b []byte) error { m[p] = bytes.Clone(b); return nil }
func clone(m memory) memory {
	n := memory{}
	for p, b := range m {
		n[p] = bytes.Clone(b)
	}
	return n
}

type Deck struct {
	ID, Name, Desc   string
	Created, Updated int64
	Cards            int
}

// Change is a candidate with exact preimages, not an acknowledgement of a write.
// Remove is distinct from an empty file. R04/R05 own source commit and journaling.
type Change struct {
	Path            string
	Before, After   []byte
	Existed, Remove bool
}
type Edit struct {
	Action, DeckID, Name, CardID, BlockID string
	Rating                                riff.Rating
	Due                                   time.Time
	DueDates                              map[string]time.Time
}
type Store struct {
	gate           chan struct{}
	once           sync.Once
	closed         bool
	source         Source
	files          memory
	decks          map[string]*riff.Deck
	settings       conf.Flashcard
	clock          func() time.Time
	rounds         map[string]*Round
	roundVersions  map[string]uint64
	pendingReviews map[string]pendingReview
}

func Open(source Source, settings conf.Flashcard, clock func() time.Time) *Store {
	if clock == nil {
		clock = time.Now
	}
	s := &Store{gate: make(chan struct{}, 1), source: source, settings: settings, clock: clock, rounds: map[string]*Round{}, roundVersions: map[string]uint64{}, pendingReviews: map[string]pendingReview{}}
	s.gate <- struct{}{}
	return s
}
func (s *Store) run(ctx context.Context, f func() error) error {
	if ctx == nil {
		return ErrInvalid
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.gate:
	}
	defer func() { s.gate <- struct{}{} }()
	if s.closed {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return f()
}

// No timer or anonymous worker survives Close. Accepted work drains before all
// deck objects, source callbacks and settings are discarded.
func (s *Store) Close() {
	s.once.Do(func() {
		<-s.gate
		defer func() { s.gate <- struct{}{} }()
		s.closed = true
		clear(s.files)
		clear(s.decks)
		clear(s.rounds)
		clear(s.roundVersions)
		clear(s.pendingReviews)
		s.source = Source{}
		s.settings = conf.Flashcard{}
	})
}

var identifier = regexp.MustCompile("^[0-9]{14}-[a-z0-9]{7}$")

func validID(id string) bool { return identifier.MatchString(id) }
func (s *Store) load(ctx context.Context) error {
	if s.source.List == nil || s.source.Read == nil {
		return ErrInvalid
	}
	names, err := s.source.List(ctx)
	if err != nil {
		return err
	}
	if len(names) > 10000 {
		return ErrInvalid
	}
	files := memory{}
	decks := map[string]*riff.Deck{}
	total := 0
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			return err
		}
		if !strings.HasSuffix(name, ".deck") {
			continue
		}
		id := strings.TrimSuffix(name, ".deck")
		if !validID(id) {
			return ErrInvalid
		}
		if _, ok := decks[id]; ok {
			return ErrInvalid
		}
		for _, p := range []string{name, id + ".cards"} {
			b, e := s.source.Read(ctx, p)
			if errors.Is(e, fs.ErrNotExist) && p != name {
				continue
			}
			if e != nil {
				return e
			}
			total += len(b)
			if total > 64<<20 {
				return ErrInvalid
			}
			files[p] = bytes.Clone(b)
		}
		d, e := s.decode(files, id)
		if e != nil {
			return e
		}
		decks[id] = d
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if s.files != nil {
		for id := range decks {
			if _, ok := s.files[id+".deck"]; ok && (!bytes.Equal(s.files[id+".deck"], files[id+".deck"]) || !bytes.Equal(s.files[id+".cards"], files[id+".cards"])) {
				delete(s.rounds, id)
				s.roundVersions[id]++
				delete(s.pendingReviews, id)
			}
		}
	}
	s.files = files
	s.decks = decks
	for id := range s.rounds {
		if decks[id] == nil {
			delete(s.rounds, id)
			delete(s.roundVersions, id)
			delete(s.pendingReviews, id)
		}
	}
	return nil
}
func (s *Store) decode(files memory, id string) (*riff.Deck, error) {
	return riff.LoadDeck(files, id, s.settings.RequestRetention, s.settings.MaximumInterval, s.settings.Weights, s.clock)
}
func (s *Store) Decks(ctx context.Context) (out []Deck, err error) {
	out = []Deck{}
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		for _, d := range s.decks {
			if d.ID != BuiltinDeckID {
				out = append(out, Deck{d.ID, d.Name, d.Desc, d.Created, d.Updated, d.CountCards()})
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Updated == out[j].Updated {
				return out[i].ID < out[j].ID
			}
			return out[i].Updated > out[j].Updated
		})
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return
}
func (s *Store) HasBuiltin(ctx context.Context) (has bool, err error) {
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		has = s.decks[BuiltinDeckID] != nil
		return nil
	})
	return
}
func (s *Store) Cards(ctx context.Context, id string, blockIDs []string) (out []riff.Card, err error) {
	out = []riff.Card{}
	blockIDs = slices.Clone(blockIDs)
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		for key, d := range s.decks {
			if id != "" && key != id {
				continue
			}
			ids := blockIDs
			if ids == nil {
				ids = d.GetBlockIDs()
			}
			for _, c := range d.GetCardsByBlockIDs(ids) {
				out = append(out, c.Clone())
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return
}

// Total counts unique bound blocks; New and Due count cards, matching the original.
func Count(deck *riff.Deck, blockIDs []string) docquery.Counts {
	if deck == nil {
		return docquery.Counts{}
	}
	ids := map[string]bool{}
	for _, id := range blockIDs {
		ids[id] = true
	}
	n := 0
	for _, id := range deck.GetBlockIDs() {
		if ids[id] {
			n++
		}
	}
	if n == 0 {
		return docquery.Counts{}
	}
	return docquery.Counts{Total: n, New: len(deck.GetNewCardsByBlockIDs(blockIDs)), Due: len(deck.GetDueCardsByBlockIDs(blockIDs))}
}
func (s *Store) Count(ctx context.Context, blockIDs []string) (out docquery.Counts, err error) {
	blockIDs = slices.Clone(blockIDs)
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		out = Count(s.decks[BuiltinDeckID], blockIDs)
		return ctx.Err()
	})
	return
}

// One immutable deck snapshot per navigation request avoids reading/redecoding
// every deck for each visible tree node. The closure cannot write source data.
func (s *Store) Counter(ctx context.Context) (counter func(context.Context, []string) (docquery.Counts, error), available bool, err error) {
	err = s.run(ctx, func() error {
		if e := s.load(ctx); e != nil {
			return e
		}
		d := s.decks[BuiltinDeckID]
		available = d != nil
		counter = func(ctx context.Context, ids []string) (docquery.Counts, error) {
			if e := ctx.Err(); e != nil {
				return docquery.Counts{}, e
			}
			out := Count(d, ids)
			return out, ctx.Err()
		}
		return nil
	})
	return
}

func (s *Store) Plan(ctx context.Context, edit Edit) (out []Change, err error) {
	err = s.run(ctx, func() error {
		if !validID(edit.DeckID) {
			return ErrInvalid
		}
		if e := s.load(ctx); e != nil {
			return e
		}
		beforeFiles := clone(s.files)
		files := clone(s.files)
		d := s.decks[edit.DeckID]
		if edit.Action == "create" {
			if d != nil {
				return fs.ErrExist
			}
		} else if d == nil {
			return fs.ErrNotExist
		}
		if edit.Action == "remove" {
			delete(files, edit.DeckID+".deck")
			delete(files, edit.DeckID+".cards")
		} else {
			var e error
			d, e = s.decode(files, edit.DeckID)
			if e != nil {
				return e
			}
			switch edit.Action {
			case "create", "rename":
				d.Name = edit.Name
			case "add":
				if !validID(edit.CardID) || !validID(edit.BlockID) {
					return ErrInvalid
				}
				if len(d.GetCardsByBlockID(edit.BlockID)) == 0 {
					d.AddCard(edit.CardID, edit.BlockID)
				}
			case "remove-card":
				if d.GetCard(edit.CardID) == nil {
					return fs.ErrNotExist
				}
				d.RemoveCard(edit.CardID)
			case "due":
				c := d.GetCard(edit.CardID)
				if c == nil {
					return fs.ErrNotExist
				}
				c.SetDue(edit.Due)
			case "due-batch":
				for id, due := range edit.DueDates {
					if !validID(id) || due.IsZero() {
						return ErrInvalid
					}
					if card := d.GetCard(id); card != nil {
						card.SetDue(due)
					}
				}
			case "review":
				if edit.Rating < riff.Again || edit.Rating > riff.Easy || d.GetCard(edit.CardID) == nil {
					return ErrInvalid
				}
				p := "logs/" + s.clock().Format("200601") + ".msgpack"
				b, e := s.source.Read(ctx, p)
				if e != nil && !errors.Is(e, fs.ErrNotExist) {
					return e
				}
				if e == nil {
					files[p] = bytes.Clone(b)
					beforeFiles[p] = bytes.Clone(b)
				}
				review, e := PrepareReview(ctx, ReviewRequest{DeckID: edit.DeckID, CardID: edit.CardID, Rating: edit.Rating, Reviewed: []string{edit.CardID}}, d, s.rounds[edit.DeckID], s.settings, func(_ context.Context, ids []string) (map[string]bool, error) {
					out := map[string]bool{}
					for _, id := range ids {
						out[id] = true
					}
					return out, nil
				})
				if e != nil {
					return e
				}
				if e = d.SaveLog(review.Log); e != nil {
					return e
				}
			default:
				return ErrInvalid
			}
			if e = d.Save(); e != nil {
				return e
			}
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		keys := map[string]bool{}
		for p := range files {
			keys[p] = true
		}
		for p := range beforeFiles {
			keys[p] = true
		}
		for p := range keys {
			before, exists := beforeFiles[p]
			after, ok := files[p]
			if exists == ok && bytes.Equal(before, after) {
				continue
			}
			out = append(out, Change{Path: p, Before: bytes.Clone(before), After: bytes.Clone(after), Existed: exists, Remove: !ok})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
		return ctx.Err()
	})
	if err != nil {
		return nil, err
	}
	return
}
