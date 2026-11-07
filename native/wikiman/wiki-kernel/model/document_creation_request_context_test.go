package model

import (
	"context"
	"errors"
	"testing"

	"wiki-kernel/internal/doclife"
)

func TestContextMutexCancellationDoesNotRetainGate(t *testing.T) {
	gate := make(contextMutex, 1)
	gate.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.LockContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lock error = %v", err)
	}
	gate.Unlock()
	if err := gate.LockContext(context.Background()); err != nil {
		t.Fatalf("gate retained after cancellation: %v", err)
	}
	gate.Unlock()
	if err := gate.LockContext(nil); err == nil || err.Error() != "document_creation_context_unbound" {
		t.Fatalf("nil context error = %v", err)
	}
}

func TestDocumentCreationContextsRejectBeforeDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CreateDocByMdContext(ctx, "box", "/doc.sy", "title", "", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("create doc error = %v", err)
	}
	if _, _, err := CreateDailyNoteContext(ctx, "box"); !errors.Is(err, context.Canceled) {
		t.Fatalf("create daily note error = %v", err)
	}
	if _, err := CreateWithMarkdownContext(ctx, "", "box", "/doc", "body", "", "", false, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("create markdown error = %v", err)
	}
	if err := SetBlockAttrsContext(ctx, "block", map[string]string{"title": "value"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("set attrs error = %v", err)
	}
	if err := BatchSetBlockAttrsContext(ctx, []map[string]interface{}{{"id": "block", "attrs": map[string]string{"title": "value"}}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("batch set attrs error = %v", err)
	}
}

func TestLegacyDocumentCreationServicesPropagateContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := captureOwnedCreationServices(ctx, context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled creation binding: %v", err)
	}
	for _, mode := range []string{"duplicate", "daily", "hpath", "markdown"} {
		if _, err := runOwnedDocumentCreation(ctx, doclife.CreationRequest{Mode: mode}); !errors.Is(err, context.Canceled) {
			t.Fatalf("%s creation error: %v", mode, err)
		}
	}
}
