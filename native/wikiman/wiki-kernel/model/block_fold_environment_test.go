package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/docquery"
)

func TestBlockFoldEnvironmentRequiresOwnerAndOuterContext(t *testing.T) {
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background()}}
	if _, e := tx.blockFold(docquery.FoldRequest{Mode: "state"}); !errors.Is(e, docquery.ErrFoldDependency) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tx.environment.ctx = ctx
	root := &ast.Node{Type: ast.NodeDocument, ID: "root"}
	root.SetIALAttr("fold", "1")
	tx.environment.blockFold = &blockFoldEnvironment{services: docquery.FoldServices{Load: func(actual context.Context, _ string) (*parse.Tree, error) {
		if actual != ctx {
			t.Fatal("outer context not forwarded")
		}
		return &parse.Tree{Root: root}, nil
	}}}
	r, e := tx.blockFold(docquery.FoldRequest{Mode: "state", ID: "root"})
	if e != nil || !r.IsRoot || !r.IsFolded {
		t.Fatal(r, e)
	}
	r, e = tx.blockFold(docquery.FoldRequest{Mode: "unfolded-parent", ID: "root"})
	if e != nil || r.ParentID != "root" {
		t.Fatal(r, e)
	}
	cancel()
	if r, e = tx.blockFold(docquery.FoldRequest{Mode: "state", ID: "root"}); !errors.Is(e, context.Canceled) || r != (docquery.FoldResult{}) {
		t.Fatal(r, e)
	}
}
