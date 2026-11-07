package model

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/internal/docquery"
)

func TestDocumentPathEnvironmentRequiresOwnerAndUsesOuterContext(t *testing.T) {
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background()}}
	if _, err := tx.documentPaths(docquery.PathRequest{Mode: "path", Path: "/"}); err == nil {
		t.Fatal("missing owner fell back")
	}
	ctx, cancel := context.WithCancel(context.Background())
	tx.environment.ctx = ctx
	tx.environment.documentPaths = &documentPathEnvironment{ctx: context.Background(), services: docquery.PathServices{LoadID: func(context.Context, string) (docquery.Location, error) {
		return docquery.Location{HPath: "owned"}, nil
	}}}
	r, err := tx.documentPaths(docquery.PathRequest{Mode: "id", ID: "block"})
	if err != nil || r.HPath != "owned" {
		t.Fatal(r, err)
	}
	cancel()
	if _, err = tx.documentPaths(docquery.PathRequest{Mode: "id"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
