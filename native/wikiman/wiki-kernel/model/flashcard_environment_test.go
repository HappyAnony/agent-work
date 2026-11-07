package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
)

func TestFlashcardEnvironmentOuterContextGateAndMissingBindings(t *testing.T) {
	ctx := context.WithValue(context.Background(), "flashcard-owner", "owner-a")
	applies := 0
	id := "20261028000000-block01"
	ports := flashcard.MutationServices{
		Decks: func(ctx context.Context) (map[string]*riff.Deck, error) {
			if ctx.Value("flashcard-owner") != "owner-a" {
				t.Fatal("lost owner")
			}
			d, e := flashcard.NewDeck(flashcard.BuiltinDeckID, "owner-a", *conf.NewFlashcard(), time.Now)
			return map[string]*riff.Deck{flashcard.BuiltinDeckID: d}, e
		},
		Load: func(context.Context, string) (*parse.Tree, error) {
			root := &ast.Node{ID: "20261028000000-doc0001", Type: ast.NodeDocument}
			root.AppendChild(&ast.Node{ID: id, Type: ast.NodeParagraph})
			return &parse.Tree{ID: root.ID, Root: root, Box: "20261028000000-box0001", Path: "/" + root.ID + ".docj"}, nil
		},
		NewID: func() string { return "20261028000000-card001" }, Apply: func(ctx context.Context, p flashcard.MutationPlan) error {
			applies++
			if ctx.Value("flashcard-owner") != "owner-a" || len(p.Trees) != 1 || p.Decks[flashcard.BuiltinDeckID].CountCards() != 1 {
				t.Fatal("lost plan/owner", p)
			}
			return nil
		},
	}
	e := &flashcardEnvironment{gate: newAttributeMutationGate(), mutation: ports}
	tx := &Transaction{environment: &transactionEnvironment{ctx: ctx, flashcard: e}}
	if result := tx.doAddFlashcards(&Operation{DeckID: flashcard.BuiltinDeckID, BlockIDs: []string{id}}); result != nil || applies != 1 {
		t.Fatal(result, applies)
	}
	tx.environment.flashcard = nil
	if result := tx.doAddFlashcards(&Operation{DeckID: flashcard.BuiltinDeckID, BlockIDs: []string{id}}); result == nil {
		t.Fatal("fell back to legacy")
	}
	if err := tx.reviewFlashcard(flashcard.ReviewRequest{}); !errors.Is(err, flashcard.ErrDependency) {
		t.Fatal(err)
	}
	tx.environment.flashcard = e
	<-e.gate.token
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	tx.environment.ctx = cancelled
	if result := tx.doRemoveFlashcards(&Operation{DeckID: flashcard.BuiltinDeckID, BlockIDs: []string{id}}); result == nil || applies != 1 {
		t.Fatal(result, applies)
	}
	e.gate.token <- struct{}{}
	tx.environment.ctx = nil
	if _, err := tx.flashcardMutation(flashcard.MutationRequest{Action: "reset"}); !errors.Is(err, flashcard.ErrDependency) {
		t.Fatal(err)
	}
}

func TestFlashcardTransactionRollbackDiscardsPostCommitEffects(t *testing.T) {
	failure := errors.New("enclosing commit failed")
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background(), finish: func(*Transaction) error { return failure }}, m: &sync.Mutex{}, flashcardAfterCommit: []flashcard.AttributeEffect{{ID: "20261028000000-block01"}}}
	tx.m.Lock()
	tx.state.Store(1)
	if err := tx.commit(); !errors.Is(err, failure) || tx.state.Load() != 3 || len(tx.flashcardAfterCommit) != 0 {
		t.Fatal(err, tx.state.Load(), tx.flashcardAfterCommit)
	}
	if !tx.m.TryLock() {
		t.Fatal("rollback did not release transaction")
	}
	tx.m.Unlock()
}

func TestFlashcardReviewEnvironmentUsesOwnerAndCancellableGate(t *testing.T) {
	ctx := context.WithValue(context.Background(), "flashcard-review", "owner-b")
	settings := *conf.NewFlashcard()
	deck, e := flashcard.NewDeck(flashcard.BuiltinDeckID, "owner-b", settings, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	cid, bid := "20261028000000-card001", "20261028000000-block01"
	deck.AddCard(cid, bid)
	deck.AddCard("20261028000000-card002", "20261028000000-block02")
	round := flashcard.NewRound()
	saves, accepts := 0, 0
	check := func(ctx context.Context) {
		if ctx.Value("flashcard-review") != "owner-b" {
			t.Fatal("review lost owner context")
		}
	}
	e2 := &flashcardEnvironment{gate: newAttributeMutationGate(), review: flashcard.ReviewServices{Settings: settings,
		Load: func(ctx context.Context, _ string) (*riff.Deck, error) {
			check(ctx)
			return flashcard.CloneDeck(deck, settings, time.Now)
		}, Round: func(ctx context.Context, _ string) (*flashcard.Round, error) { check(ctx); return round, nil },
		Exists: func(ctx context.Context, ids []string) (map[string]bool, error) {
			check(ctx)
			out := map[string]bool{}
			for _, id := range ids {
				out[id] = true
			}
			return out, nil
		},
		Save: func(ctx context.Context, p flashcard.ReviewPlan) error {
			check(ctx)
			saves++
			deck = p.Deck
			return nil
		}, Accept: func(ctx context.Context, _ string, r *flashcard.Round) error {
			check(ctx)
			accepts++
			round = r
			return nil
		}}}
	tx := &Transaction{environment: &transactionEnvironment{ctx: ctx, flashcard: e2}}
	q := flashcard.ReviewRequest{DeckID: flashcard.BuiltinDeckID, CardID: cid, Rating: riff.Good, Reviewed: []string{cid}}
	if e = tx.reviewFlashcard(q); e != nil || saves != 1 || accepts != 1 {
		t.Fatal(e, saves, accepts)
	}
	<-e2.gate.token
	cancelled, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	tx.environment.ctx = cancelled
	if e = tx.reviewFlashcard(q); !errors.Is(e, context.DeadlineExceeded) || saves != 1 || accepts != 1 {
		t.Fatal(e, saves, accepts)
	}
	e2.gate.token <- struct{}{}
}
