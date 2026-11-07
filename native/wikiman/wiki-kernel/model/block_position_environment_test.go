package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/docquery"
)

func TestBlockPositionEnvironmentRequiresOwnerAndOuterContext(t *testing.T) {
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background()}}
	if _, e := tx.blockPosition(docquery.PositionRequest{Mode: "indexes"}); !errors.Is(e, docquery.ErrPositionDependency) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tx.environment.ctx = ctx
	root := &ast.Node{Type: ast.NodeDocument, ID: "root"}
	root.AppendChild(&ast.Node{Type: ast.NodeParagraph, ID: "block"})
	tx.environment.blockPosition = &blockPositionEnvironment{ctx: context.Background(), services: docquery.PositionServices{Load: func(c context.Context, _ string) (*parse.Tree, error) {
		if c != ctx {
			t.Fatal("outer context not forwarded")
		}
		return &parse.Tree{Root: root}, nil
	}}}
	r, e := tx.blockPosition(docquery.PositionRequest{Mode: "index", ID: "block"})
	if e != nil || r.Index != 1 {
		t.Fatal(r, e)
	}
	cancel()
	if _, e = tx.blockPosition(docquery.PositionRequest{Mode: "index", ID: "block"}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
