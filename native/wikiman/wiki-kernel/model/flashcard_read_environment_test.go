package model

import (
	"context"
	"errors"
	"testing"
	"time"
	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
	"wiki-kernel/internal/indexdb"
)

func TestFlashcardReadEnvironmentRequiresOwnerAndPropagatesContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), "card-read-owner", "owner")
	check := func(ctx context.Context) {
		if ctx.Value("card-read-owner") != "owner" {
			t.Fatal("lost owner")
		}
	}
	calls := 0
	e := &flashcardEnvironment{gate: newAttributeMutationGate(), read: flashcard.ReadServices{Cards: func(ctx context.Context, _ string, _ string, _ []string) ([]riff.Card, bool, error) {
		check(ctx)
		calls++
		return nil, false, nil
	}, Blocks: func(context.Context, []string) ([]*indexdb.BlockRecord, error) {
		t.Fatal("absent deck queried SQL")
		return nil, nil
	}, Missing: func() string { return "missing" }, Now: time.Now}, dueView: flashcard.DueViewServices{Due: func(ctx context.Context, q flashcard.DueRequest) (flashcard.DueResult, error) {
		check(ctx)
		return flashcard.DueResult{}, nil
	}, Format: func(time.Time, time.Time) string { return "duration" }, Now: time.Now}}
	tx := &Transaction{environment: &transactionEnvironment{ctx: ctx, flashcard: e}}
	q := flashcard.PageRequest{Scope: "deck", Page: 1, PageSize: 20}
	if p, err := tx.flashcardPage(q); err != nil || len(p.Blocks) != 0 || calls != 1 {
		t.Fatal(p, err, calls)
	}
	if _, err := tx.flashcardDueView(flashcard.DueRequest{}); err != nil {
		t.Fatal(err)
	}
	tx.environment.flashcard = nil
	if _, err := tx.flashcardPage(q); !errors.Is(err, flashcard.ErrDependency) {
		t.Fatal("legacy fallback", err)
	}
	tx.environment.flashcard = e
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	tx.environment.ctx = cancelled
	if _, err := tx.flashcardPage(q); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatal(err, calls)
	}
}
