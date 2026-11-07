package flashcard

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexdb"
)

func presentationSource(t *testing.T) ([]riff.Card, ReadServices) {
	t.Helper()
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	d, e := NewDeck(BuiltinDeckID, "view", *conf.NewFlashcard(), func() time.Time { return now })
	if e != nil {
		t.Fatal(e)
	}
	for i, id := range []string{blockID, mutationBlock2} {
		cid := fmt.Sprintf("2026102800000%d-card001", i)
		d.AddCard(cid, id)
		d.GetCard(cid).SetDue(now.Add(time.Duration(2-i) * time.Hour))
	}
	cards := d.GetCardsByBlockIDs(d.GetBlockIDs())
	return cards, ReadServices{
		Cards: func(context.Context, string, string, []string) ([]riff.Card, bool, error) { return cards, true, nil }, Blocks: func(_ context.Context, ids []string) ([]*indexdb.BlockRecord, error) {
			out := []*indexdb.BlockRecord{}
			for i := len(ids) - 1; i >= 0; i-- {
				if ids[i] == blockID {
					out = append(out, &indexdb.BlockRecord{ID: blockID, Box: "owner", Type: "p", Content: "<text> & value", IAL: `{: id="` + blockID + `"}`})
				}
			}
			return out, nil
		}, Missing: func() string { return "owner missing" }, Now: func() time.Time { return now },
	}
}
func TestFlashcardPageProjectionOrderPlaceholdersAndBorrowedData(t *testing.T) {
	ctx := context.Background()
	cards, s := presentationSource(t)
	original := cards[0].ID()
	p, e := ReadPage(ctx, PageRequest{Scope: "deck", Page: 1, PageSize: 1}, s)
	if e != nil || p.Total != 2 || p.PageCount != 2 || len(p.Blocks) != 1 || p.Blocks[0].ID != mutationBlock2 || p.Blocks[0].Content != "owner missing" || p.Blocks[0].RiffCard != nil {
		t.Fatal(p, e)
	}
	p, e = ReadPage(ctx, PageRequest{Scope: "deck", Page: 2, PageSize: 1}, s)
	if e != nil || p.Blocks[0].ID != blockID || p.Blocks[0].RiffCard == nil || p.Blocks[0].RiffCardID == "" || p.Blocks[0].Content != "&lt;text&gt; &amp; value" {
		t.Fatal(p, e)
	}
	if cards[0].ID() != original {
		t.Fatal("sorted borrowed slice")
	}
	p, e = ReadPage(ctx, PageRequest{Scope: "blocks", IDs: []string{blockID, "missing", blockID}}, s)
	if e != nil || len(p.Blocks) != 3 || p.Blocks[1].ID != "missing" || p.Blocks[2].RiffCardID != p.Blocks[0].RiffCardID {
		t.Fatal(p, e)
	}
	p.Blocks[0].RiffCard.Reps = 123
	if p.Blocks[2].RiffCard.Reps == 123 {
		t.Fatal("duplicate result aliases")
	}
	for _, q := range []PageRequest{{Scope: "deck", Page: math.MaxInt, PageSize: math.MaxInt}, {Scope: "deck", Page: 2, PageSize: math.MaxInt}} {
		p, e = ReadPage(ctx, q, s)
		if e != nil || len(p.Blocks) != 0 || p.Total != 2 || p.PageCount != 1 {
			t.Fatal(p, e)
		}
	}
	for _, q := range []PageRequest{{Scope: "deck", Page: 0, PageSize: 1}, {Scope: "deck", Page: 1, PageSize: 0}, {Scope: "unknown", Page: 1, PageSize: 1}} {
		if _, e = ReadPage(ctx, q, s); !errors.Is(e, ErrInvalid) {
			t.Fatal(q, e)
		}
	}
}
func TestFlashcardPageBatchesAndRejectsForeignRowsOrLateCancellation(t *testing.T) {
	ctx := context.Background()
	cards, s := presentationSource(t)
	many := make([]riff.Card, 1100)
	for i := range many {
		many[i] = cards[0]
	}
	calls := 0
	s.Blocks = func(_ context.Context, ids []string) ([]*indexdb.BlockRecord, error) {
		calls++
		if len(ids) > 512 {
			t.Fatal("oversized SQL batch")
		}
		return nil, nil
	}
	p, e := ProjectPage(ctx, many, 1, len(many), s)
	if e != nil || calls != 3 || len(p.Blocks) != 1100 {
		t.Fatal(calls, len(p.Blocks), e)
	}
	s.Blocks = func(context.Context, []string) ([]*indexdb.BlockRecord, error) {
		return []*indexdb.BlockRecord{{ID: "foreign"}}, nil
	}
	if _, e = ProjectPage(ctx, cards, 1, 20, s); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	s.Blocks = func(context.Context, []string) ([]*indexdb.BlockRecord, error) { cancel(); return nil, nil }
	if _, e = ProjectPage(cancelled, cards, 1, 20, s); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestFlashcardDocumentLimitsAndDueProjection(t *testing.T) {
	c := *conf.NewFlashcard()
	overflow := "9999999999999999999999999"
	wantOverflow, _ := strconv.Atoi(overflow)
	limits := DocumentLimits(c, map[string]string{"custom-riff-review-card-limit": overflow})
	if limits.ReviewCardLimit != wantOverflow {
		t.Fatal(limits, wantOverflow)
	}
	for _, v := range []string{"0", "-1", "garbage", "2"} {
		got := DocumentLimits(c, map[string]string{"custom-riff-new-card-limit": v})
		want := 0
		if v == "-1" {
			want = -1
		}
		if v == "2" {
			want = 2
		}
		if got.NewCardLimit != want || got.ReviewCardLimit != c.ReviewCardLimit || c.NewCardLimit != 20 {
			t.Fatal(got, c)
		}
	}
	cards, s := presentationSource(t)
	cards[0].SetNextDues(map[riff.Rating]time.Time{riff.Good: s.Now().Add(time.Hour)})
	view, e := ReadDueView(context.Background(), DueRequest{}, DueViewServices{Due: func(_ context.Context, q DueRequest) (DueResult, error) {
		if q.DeckID != BuiltinDeckID {
			t.Fatal(q)
		}
		return DueResult{Cards: cards, New: 2, Unreviewed: 2}, nil
	}, Now: s.Now, Format: func(time.Time, time.Time) string { return "  one hour  " }})
	if e != nil || len(view.Cards) != 2 || view.Cards[0].NextDues[riff.Good] != "one hour" || view.New != 2 {
		t.Fatal(view, e)
	}
}
