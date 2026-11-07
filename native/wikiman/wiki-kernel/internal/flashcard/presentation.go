// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package flashcard

import (
	"context"
	"github.com/open-spaced-repetition/go-fsrs/v3"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexdb"
)

type PageRequest struct {
	Scope, ID      string
	IDs            []string
	Page, PageSize int
}
type PageResult struct {
	Blocks    []*blockview.Block `json:"blocks"`
	Total     int                `json:"total"`
	PageCount int                `json:"pageCount"`
}
type ReadServices struct {
	Cards         func(context.Context, string, string, []string) ([]riff.Card, bool, error)
	Blocks        func(context.Context, []string) ([]*indexdb.BlockRecord, error)
	Missing       func() string
	Now           func() time.Time
	CaseSensitive bool
}

func ReadPage(ctx context.Context, q PageRequest, s ReadServices) (PageResult, error) {
	if ctx == nil || s.Cards == nil || s.Blocks == nil || s.Missing == nil || s.Now == nil {
		return PageResult{}, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return PageResult{}, e
	}
	if q.Scope != "blocks" && q.Scope != "deck" && q.Scope != "tree" && q.Scope != "notebook" {
		return PageResult{}, ErrInvalid
	}
	if q.Scope != "blocks" && (q.Page < 1 || q.PageSize < 1) {
		return PageResult{}, ErrInvalid
	}
	q.IDs = slices.Clone(q.IDs)
	cards, available, e := s.Cards(ctx, q.Scope, q.ID, q.IDs)
	if e != nil {
		return PageResult{}, e
	}
	if e = ctx.Err(); e != nil {
		return PageResult{}, e
	}
	if !available {
		return PageResult{Blocks: []*blockview.Block{}}, nil
	}
	if q.Scope == "blocks" {
		p, e := ProjectPage(ctx, cards, 1, max(1, len(cards)), s)
		if e != nil {
			return PageResult{}, e
		}
		byID := map[string]*blockview.Block{}
		for _, b := range p.Blocks {
			if byID[b.ID] == nil {
				byID[b.ID] = b
			}
		}
		out := PageResult{Blocks: []*blockview.Block{}}
		for _, id := range q.IDs {
			b := byID[id]
			if b == nil {
				b = &blockview.Block{ID: id, Content: s.Missing()}
			}
			copy := *b
			copy.IAL = maps.Clone(b.IAL)
			if b.RiffCard != nil {
				card := *b.RiffCard
				copy.RiffCard = &card
			}
			out.Blocks = append(out.Blocks, &copy)
		}
		out.Total = len(out.Blocks)
		if out.Total > 0 {
			out.PageCount = 1
		}
		return out, ctx.Err()
	}
	return ProjectPage(ctx, cards, q.Page, q.PageSize, s)
}

// Sort copies the caller's slice and preserves original due/zero-date ID rules.
// Integer arithmetic avoids page-count rounding and multiplication overflow.
func ProjectPage(ctx context.Context, cards []riff.Card, page, size int, s ReadServices) (out PageResult, err error) {
	if ctx == nil || s.Blocks == nil || s.Missing == nil || s.Now == nil {
		return out, ErrDependency
	}
	if page < 1 || size < 1 {
		return out, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	cards = slices.Clone(cards)
	for _, c := range cards {
		if c == nil {
			return out, ErrInvalid
		}
		if f, ok := c.(*riff.FSRSCard); !ok || f == nil || f.C == nil || f.BaseCard == nil {
			return out, ErrInvalid
		}
	}
	sort.SliceStable(cards, func(i, j int) bool {
		a, b := cards[i].(*riff.FSRSCard).C.Due, cards[j].(*riff.FSRSCard).C.Due
		if a.IsZero() || b.IsZero() {
			return cards[i].ID() < cards[j].ID()
		}
		if a.Equal(b) {
			return cards[i].ID() < cards[j].ID()
		}
		return a.Before(b)
	})
	out = PageResult{Blocks: []*blockview.Block{}, Total: len(cards)}
	if len(cards) == 0 {
		return out, nil
	}
	out.PageCount = 1 + (len(cards)-1)/size
	if page > out.PageCount {
		return out, nil
	}
	start := (page - 1) * size
	end := start + min(size, len(cards)-start)
	selected := cards[start:end]
	ids := []string{}
	for _, c := range selected {
		ids = append(ids, c.BlockID())
	}
	rows := map[string]*indexdb.BlockRecord{}
	for start := 0; start < len(ids); start += 512 {
		if e := ctx.Err(); e != nil {
			return PageResult{}, e
		}
		batch := ids[start:min(start+512, len(ids))]
		got, e := s.Blocks(ctx, slices.Clone(batch))
		if e != nil {
			return PageResult{}, e
		}
		if e = ctx.Err(); e != nil {
			return PageResult{}, e
		}
		for _, row := range got {
			if row != nil {
				if !slices.Contains(batch, row.ID) {
					return PageResult{}, ErrInvalid
				}
				copy := *row
				rows[row.ID] = &copy
			}
		}
	}
	now := s.Now()
	for _, c := range selected {
		row := rows[c.BlockID()]
		b := blockview.FromRow(row, "", 36, s.CaseSensitive)
		if b == nil {
			b = &blockview.Block{ID: c.BlockID(), Content: s.Missing()}
		} else {
			b.RiffCardID = c.ID()
			b.RiffCard = CardView(c.(*riff.FSRSCard).C, now)
		}
		out.Blocks = append(out.Blocks, b)
	}
	if e := ctx.Err(); e != nil {
		return PageResult{}, e
	}
	return out, nil
}
func CardView(c *fsrs.Card, now time.Time) *blockview.RiffCard {
	due := c.Due
	if due.IsZero() {
		due = now
	}
	return &blockview.RiffCard{Due: due, Reps: c.Reps, Lapses: c.Lapses, State: c.State, LastReview: c.LastReview}
}

// Invalid/overflow values retain strconv.Atoi's original result (including 0
// for malformed text); absent/empty attributes retain the workspace defaults.
func DocumentLimits(c conf.Flashcard, attrs map[string]string) conf.Flashcard {
	if s := attrs["custom-riff-new-card-limit"]; s != "" {
		c.NewCardLimit, _ = strconv.Atoi(s)
	}
	if s := attrs["custom-riff-review-card-limit"]; s != "" {
		c.ReviewCardLimit, _ = strconv.Atoi(s)
	}
	return c
}

type DueCard struct {
	DeckID     string                 `json:"deckID"`
	CardID     string                 `json:"cardID"`
	BlockID    string                 `json:"blockID"`
	Lapses     int                    `json:"lapses"`
	Reps       int                    `json:"reps"`
	State      riff.State             `json:"state"`
	LastReview int64                  `json:"lastReview"`
	NextDues   map[riff.Rating]string `json:"nextDues"`
}

type DueRequest struct {
	DeckID, Scope, ID string
	Reviewed          []string
}
type DueView struct {
	Cards      []*DueCard `json:"cards"`
	Unreviewed int        `json:"unreviewedCount"`
	New        int        `json:"unreviewedNewCardCount"`
	Old        int        `json:"unreviewedOldCardCount"`
}
type DueViewServices struct {
	Due    func(context.Context, DueRequest) (DueResult, error)
	Format func(time.Time, time.Time) string
	Now    func() time.Time
}

func ReadDueView(ctx context.Context, q DueRequest, s DueViewServices) (DueView, error) {
	if ctx == nil || s.Due == nil || s.Format == nil || s.Now == nil {
		return DueView{}, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return DueView{}, e
	}
	if q.DeckID == "" {
		q.DeckID = BuiltinDeckID
	}
	q.Reviewed = slices.Clone(q.Reviewed)
	due, e := s.Due(ctx, q)
	if e != nil {
		return DueView{}, e
	}
	if e = ctx.Err(); e != nil {
		return DueView{}, e
	}
	out := DueView{Cards: []*DueCard{}, Unreviewed: due.Unreviewed, New: due.New, Old: due.Old}
	now := s.Now()
	for _, c := range due.Cards {
		if c == nil {
			return DueView{}, ErrInvalid
		}
		out.Cards = append(out.Cards, DueCardView(c, q.DeckID, now, s.Format))
	}
	if e = ctx.Err(); e != nil {
		return DueView{}, e
	}
	return out, nil
}

func DueCardView(c riff.Card, deck string, now time.Time, format func(time.Time, time.Time) string) *DueCard {
	next := map[riff.Rating]string{}
	for r, due := range c.NextDues() {
		next[r] = strings.TrimSpace(format(due, now))
	}
	return &DueCard{DeckID: deck, CardID: c.ID(), BlockID: c.BlockID(), Lapses: c.GetLapses(), Reps: c.GetReps(), State: c.GetState(), LastReview: c.GetLastReview().UnixMilli(), NextDues: next}
}
