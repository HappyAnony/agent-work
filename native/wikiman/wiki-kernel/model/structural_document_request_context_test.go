package model

import (
	"context"
	"errors"
	"testing"
)

func TestStructuralDocumentConversionsRejectCanceledContextBeforeDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := Heading2DocContext(ctx, "heading", "box", "/", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("heading error = %v", err)
	}
	if _, _, err := ListItem2DocContext(ctx, "item", "box", "/", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("list item error = %v", err)
	}
	if _, _, err := Heading2DocContext(nil, "heading", "box", "/", ""); err == nil || err.Error() != "structural_document_context_unbound" {
		t.Fatalf("nil context error = %v", err)
	}
}

func TestStructuralDocumentOwnerRejectsUnboundFollowup(t *testing.T) {
	if err := (*structuralDocumentOwner)(nil).scheduleReferenceRefresh("source", "target"); err == nil || err.Error() != "structural_document_owner_unbound" {
		t.Fatalf("followup error = %v", err)
	}
	if err := (*structuralDocumentOwner)(nil).scheduleReferenceRefreshAfter(1, "source", "target"); err == nil || err.Error() != "structural_document_owner_unbound" {
		t.Fatalf("delayed followup error = %v", err)
	}
}
