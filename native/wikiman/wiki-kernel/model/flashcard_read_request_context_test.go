package model

import (
	"context"
	"errors"
	"testing"
)

func TestFlashcardReadContextsRejectBeforeLegacyDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		run  func() error
	}{
		{"blocks", func() error { _, err := GetFlashcardsByBlockIDsContext(ctx, nil); return err }},
		{"notebook-page", func() error { _, _, _, err := GetNotebookFlashcardsContext(ctx, "box", 1, 20); return err }},
		{"tree-page", func() error { _, _, _, err := GetTreeFlashcardsContext(ctx, "tree", 1, 20); return err }},
		{"deck-page", func() error { _, _, _, err := GetDeckFlashcardsContext(ctx, "deck", 1, 20); return err }},
		{"notebook-due", func() error { _, _, _, _, err := GetNotebookDueFlashcardsContext(ctx, "box", nil); return err }},
		{"tree-due", func() error { _, _, _, _, err := GetTreeDueFlashcardsContext(ctx, "tree", nil); return err }},
		{"deck-due", func() error { _, _, _, _, err := GetDueFlashcardsContext(ctx, "deck", nil); return err }},
		{"decks", func() error { _, err := GetDecksContext(ctx); return err }},
		{"notebooks", func() error { _, err := GetFlashcardNotebooksContext(ctx); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v", err)
			}
		})
	}

	if _, err := GetDecksContext(nil); err == nil || err.Error() != "flashcard_context_unbound" {
		t.Fatalf("nil context error = %v", err)
	}
}
