package model

import (
	"context"
	"testing"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/tagquery"
)

func TestTagNavigationTransactionContextNoGlobalFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	e := &tagNavigationEnvironment{ctx: context.Background(), limit: 9, services: tagquery.NavigationServices{
		Spans: func(ctx context.Context, _ string, limit int) ([]indexdb.SpanRecord, error) {
			calls++
			if limit != 9 {
				t.Fatal(limit)
			}
			return []indexdb.SpanRecord{{Content: "tag", BlockID: "id"}}, ctx.Err()
		},
		Blocks: func(context.Context, []string) ([]*indexdb.BlockRecord, error) {
			return []*indexdb.BlockRecord{{ID: "id", Content: "owner", Type: "p"}}, nil
		},
	}}
	tx := &Transaction{environment: &transactionEnvironment{ctx: ctx, tagNavigation: e}}
	got, err := tx.tagBlocksByKeyword("tag")
	if err != nil || got["tag"][0].Content != "owner" {
		t.Fatal(got, err)
	}
	cancel()
	if got, err = tx.tagBlocksByKeyword("tag"); err == nil || got != nil || calls != 1 {
		t.Fatal("outer ctx lost", got, err, calls)
	}
	tx.environment.tagNavigation = nil
	if got, err = tx.tagBlocksByKeyword("tag"); err == nil || got != nil {
		t.Fatal("global fallback", got, err)
	}
}
