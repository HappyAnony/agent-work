package model

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/internal/doctree"
)

func TestDocumentTreeEnvironmentOuterContextAndNoGlobalFallback(t *testing.T) {
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background()}}
	if _, _, err := tx.listDocumentTree(doctree.Request{}); err == nil {
		t.Fatal("missing environment fell back")
	}
	tx.environment.documentTree = &documentTreeEnvironment{ctx: context.Background()}
	if _, _, err := tx.listDocumentTree(doctree.Request{}); err == nil {
		t.Fatal("missing services accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx.environment.ctx = ctx
	s := doctree.Services{Settings: func(context.Context, string) (int, int, error) {
		t.Fatal("cancelled owner read settings")
		return 0, 0, nil
	}, List: func(context.Context, string) ([]doctree.Entry, error) { return nil, nil }, Stat: func(context.Context, string) (*doctree.Entry, error) { return nil, nil }, IAL: func(context.Context, string) (map[string]string, error) { return nil, nil }, Children: func(context.Context, string) ([]doctree.Entry, error) { return nil, nil }, IsDir: func(context.Context, string) (bool, error) { return false, nil }, Project: func(context.Context, doctree.Entry, map[string]string) (*File, error) { return nil, nil }, References: func(context.Context) (map[string]int, error) { return nil, nil }, Sorts: func(context.Context, []*File) error { return nil }}
	tx.environment.documentTree.services = s
	if _, _, err := tx.listDocumentTree(doctree.Request{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
