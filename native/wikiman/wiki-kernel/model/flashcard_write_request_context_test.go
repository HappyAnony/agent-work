package model

import (
	"context"
	"errors"
	"testing"

	"wiki-kernel/internal/flashcard"
	"wiki-kernel/internal/flashcard/riff"
)

func TestFlashcardWriteContextsRejectBeforeLegacyDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		run  func() error
	}{
		{"due", func() error { return SetFlashcardsDueTimeContext(ctx, nil) }},
		{"reset", func() error { return ResetFlashcardsContext(ctx, "deck", "deck", "deck", nil) }},
		{"review", func() error { return ReviewFlashcardContext(ctx, "deck", "card", riff.Again, nil) }},
		{"skip", func() error { return SkipReviewFlashcardContext(ctx, "deck", "card") }},
		{"rename", func() error { return RenameDeckContext(ctx, "deck", "name") }},
		{"remove", func() error { return RemoveDeckContext(ctx, "deck") }},
		{"create", func() error { _, err := CreateDeckContext(ctx, "name"); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	if err := SetFlashcardsDueTimeContext(nil, nil); err == nil || err.Error() != "flashcard_context_unbound" {
		t.Fatalf("nil context error = %v", err)
	}
}

func TestLegacyFlashcardTransactionUsesAdmissionContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx := &Transaction{environment: &transactionEnvironment{ctx: ctx, legacy: true}}
	if result := tx.doAddFlashcards(&Operation{DeckID: flashcard.BuiltinDeckID, BlockIDs: []string{"block"}}); result == nil || result.msg != context.Canceled.Error() {
		t.Fatalf("transaction result = %#v", result)
	}
	if deckLock.TryLock() {
		deckLock.Unlock()
	} else {
		t.Fatal("canceled transaction retained deck lock")
	}
}
