package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/docquery"
)

func TestBlockRelationEnvironmentRequiresOwnerAndOuterContext(t *testing.T) {
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background()}}
	if _, e := tx.blockRelations(docquery.RelationRequest{Mode: "infos"}); !errors.Is(e, docquery.ErrRelationDependency) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tx.environment.ctx = ctx
	root := &ast.Node{Type: ast.NodeDocument, ID: "root"}
	root.AppendChild(&ast.Node{Type: ast.NodeParagraph, ID: "block"})
	tree := &parse.Tree{Root: root}
	check := func(actual context.Context) {
		if actual != ctx {
			t.Fatal("outer context not forwarded")
		}
	}
	tx.environment.blockRelations = &blockRelationEnvironment{ctx: context.Background(), services: docquery.RelationServices{Trees: func(c context.Context, _ []string) (map[string]*parse.Tree, error) {
		check(c)
		return map[string]*parse.Tree{"block": tree}, nil
	}, Load: func(c context.Context, _ string) (*parse.Tree, error) { check(c); return tree, nil }, BlockMissing: ErrBlockNotFound}}
	r, e := tx.blockRelations(docquery.RelationRequest{Mode: "infos", IDs: []string{"block"}})
	if e != nil || r.Infos["block"].ParentID != "root" {
		t.Fatal(r, e)
	}
	r, e = tx.blockRelations(docquery.RelationRequest{Mode: "relevant", ID: "block"})
	if e != nil || r.IDs.Parent != "root" {
		t.Fatal(r, e)
	}
	cancel()
	if _, e = tx.blockRelations(docquery.RelationRequest{Mode: "sibling", ID: "block"}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
