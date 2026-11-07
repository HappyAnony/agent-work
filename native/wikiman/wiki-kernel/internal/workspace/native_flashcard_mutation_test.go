//go:build fts5

package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
)

func TestNativeFlashcardMutationJointCandidatesSameIDsAndEffects(t *testing.T) {
	ctx := context.Background()
	root := flashSource(t)
	before := digest(t, root)
	m := open(t, root, flashOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		for _, action := range []string{"add", "remove", "reset"} {
			q := flashcard.MutationRequest{Action: action, DeckID: flashcard.BuiltinDeckID, BlockIDs: []string{blockID}}
			if action == "add" {
				q.BlockIDs = append(q.BlockIDs, positionInside)
			}
			p, e := l.PlanNativeFlashcardMutation(ctx, q)
			if e != nil || len(p.Trees) != 1 || len(p.SourceHashes) != 1 || len(p.Changes) == 0 {
				t.Fatal(action, p, e)
			}
			if p.Trees[0].Root.IALAttr("title") != "Title "+l.PageID() {
				t.Fatal("foreign tree", p.Trees[0])
			}
			d := p.Decks[flashcard.BuiltinDeckID]
			want := 1
			if action == "add" {
				want = 2
			}
			if action == "remove" {
				want = 0
			}
			if d == nil || d.CountCards() != want {
				t.Fatal(action, d, want)
			}
			if action == "reset" && d.GetCard(flashCardID) != nil {
				t.Fatal("old learning state survived reset")
			}
			for _, effect := range p.Attributes {
				wantAttr := flashcard.BuiltinDeckID
				if action == "remove" {
					wantAttr = ""
				}
				if effect.New["custom-riff-decks"] != wantAttr {
					t.Fatal(effect)
				}
			}
		}
	}
	if digest(t, root) != before {
		t.Fatal("joint preparation wrote source/cache/notifications")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := a.PlanNativeFlashcardMutation(cancelled, flashcard.MutationRequest{Action: "add"}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	a.Release()
	if _, e := a.PlanNativeFlashcardMutation(ctx, flashcard.MutationRequest{Action: "reset"}); e == nil {
		t.Fatal("released owner prepared")
	}
}
func addSecondReviewCard(t *testing.T, root string) {
	t.Helper()
	c := conf.NewFlashcard()
	for _, page := range []string{pageA, pageB} {
		dir := filepath.Join(root, "pages", page, "data", "storage", "riff")
		files := flashMemory{}
		for _, ext := range []string{".deck", ".cards"} {
			b, e := os.ReadFile(filepath.Join(dir, flashcard.BuiltinDeckID+ext))
			if e != nil {
				t.Fatal(e)
			}
			files[flashcard.BuiltinDeckID+ext] = b
		}
		d, e := riff.LoadDeck(files, flashcard.BuiltinDeckID, c.RequestRetention, c.MaximumInterval, c.Weights, time.Now)
		if e != nil {
			t.Fatal(e)
		}
		d.AddCard("20261028000000-card002", positionInside)
		if e = d.Save(); e != nil {
			t.Fatal(e)
		}
		for p, b := range files {
			if e = os.WriteFile(filepath.Join(dir, p), b, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
}

// This helper simulates the later commit adapter against disposable fixtures.
// It is intentionally not production source publication or journal acceptance.
func publishReviewFixture(t *testing.T, root, page string, p flashcard.PreparedReview) {
	t.Helper()
	for _, c := range p.Changes {
		file := filepath.Join(root, "pages", page, "data", filepath.FromSlash(c.Path))
		if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(file, c.After, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestNativeFlashcardReviewConfirmationAndRepeatedUndoOwnerIsolation(t *testing.T) {
	ctx := context.Background()
	root := flashSource(t)
	addSecondReviewCard(t, root)
	m := open(t, root, flashOptions(root))
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	before := digest(t, root)
	q := flashcard.ReviewRequest{DeckID: flashcard.BuiltinDeckID, CardID: flashCardID, Rating: riff.Good, Reviewed: []string{flashCardID}}
	prepared, e := a.PrepareNativeFlashcardReview(ctx, q)
	if e != nil || prepared.Token == "" || len(prepared.Changes) < 2 {
		t.Fatal(prepared, e)
	}
	if digest(t, root) != before {
		t.Fatal("prepare wrote source")
	}
	if e = a.ConfirmNativeFlashcardReview(ctx, prepared.Token); e == nil {
		t.Fatal("uncommitted review accepted")
	}
	if e = b.ConfirmNativeFlashcardReview(ctx, prepared.Token); !errors.Is(e, flashcard.ErrReviewConflict) {
		t.Fatal("foreign token", e)
	}
	publishReviewFixture(t, root, pageA, prepared)
	if e = a.ConfirmNativeFlashcardReview(ctx, prepared.Token); e != nil {
		t.Fatal(e)
	}
	for _, rating := range []riff.Rating{riff.Again, riff.Easy, riff.Hard} {
		q.Rating = rating
		p, e := a.PrepareNativeFlashcardReview(ctx, q)
		if e != nil {
			t.Fatal(e)
		}
		publishReviewFixture(t, root, pageA, p)
		if e = a.ConfirmNativeFlashcardReview(ctx, p.Token); e != nil {
			t.Fatal(e)
		}
		cards, e := a.NativeFlashcardCards(ctx, flashcard.BuiltinDeckID, nil)
		if e != nil {
			t.Fatal(e)
		}
		for _, card := range cards {
			if card.ID() == flashCardID && card.GetReps() != 1 {
				t.Fatal("undo baseline overwritten", card)
			}
		}
	}
	cards, e := b.NativeFlashcardCards(ctx, flashcard.BuiltinDeckID, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range cards {
		if c.ID() == flashCardID && c.GetReps() != 0 {
			t.Fatal("foreign learning state")
		}
	}
	pending, e := a.PrepareNativeFlashcardReview(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if e = a.ConfirmNativeFlashcardReview(cancelled, pending.Token); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = a.AbortNativeFlashcardReview(ctx, pending.Token); e != nil {
		t.Fatal(e)
	}
	if e = a.ConfirmNativeFlashcardReview(ctx, pending.Token); !errors.Is(e, flashcard.ErrReviewConflict) {
		t.Fatal(e)
	}
}

func TestNativeFlashcardBuiltinCreationDoesNotClobberOrphanCardFile(t *testing.T) {
	ctx := context.Background()
	root := flashSource(t)
	dir := filepath.Join(root, "pages", pageA, "data", "storage", "riff")
	cardfile := filepath.Join(dir, flashcard.BuiltinDeckID+".cards")
	original, e := os.ReadFile(cardfile)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.RemoveAll(dir); e != nil {
		t.Fatal(e)
	}
	m := open(t, root, flashOptions(root))
	l := getDocLease(t, m, pageA)
	before := digest(t, root)
	q := flashcard.MutationRequest{Action: "add", DeckID: flashcard.BuiltinDeckID, BlockIDs: []string{blockID}}
	plan, e := l.PlanNativeFlashcardMutation(ctx, q)
	if e != nil || len(plan.Changes) != 2 || plan.Decks[flashcard.BuiltinDeckID].CountCards() != 1 {
		t.Fatal(plan, e)
	}
	if digest(t, root) != before {
		t.Fatal("preview created default storage")
	}
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(cardfile, original, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = l.PlanNativeFlashcardMutation(ctx, q); !errors.Is(e, flashcard.ErrReviewConflict) {
		t.Fatal("orphan source would be overwritten", e)
	}
}
