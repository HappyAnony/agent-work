package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/docquery"
)

func TestDocumentInfoEnvironmentRequiresOwnerAndUsesOuterContext(t *testing.T) {
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background()}}
	if _, e := tx.documentInfos(docquery.InfoRequest{Batch: true}); !errors.Is(e, docquery.ErrInfoDependency) {
		t.Fatal("implicit global fallback", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tx.environment.ctx = ctx
	check := func(actual context.Context) {
		if actual != ctx {
			t.Fatal("outer context not forwarded")
		}
	}
	tree := &parse.Tree{Root: &ast.Node{Type: ast.NodeDocument, ID: "root"}}
	tree.Root.SetIALAttr("title", "owned")
	tx.environment.documentInfo = &documentInfoEnvironment{ctx: context.Background(), services: docquery.InfoServices{Wait: func(c context.Context) error { check(c); return nil }, Trees: func(c context.Context, _ []string) (map[string]*parse.Tree, error) {
		check(c)
		return map[string]*parse.Tree{"block": tree}, nil
	}, Exists: func(c context.Context, _ string) (bool, error) { check(c); return true, nil }, Subfiles: func(c context.Context, _ *parse.Tree) (int, error) { check(c); return 0, nil }}}
	rows, e := tx.documentInfos(docquery.InfoRequest{Batch: true, IDs: []string{"block"}})
	if e != nil || len(rows) != 1 || rows[0].Name != "owned" {
		t.Fatal(rows, e)
	}
	cancel()
	if rows, e = tx.documentInfos(docquery.InfoRequest{Batch: true}); !errors.Is(e, context.Canceled) || rows != nil {
		t.Fatal(rows, e)
	}
}
