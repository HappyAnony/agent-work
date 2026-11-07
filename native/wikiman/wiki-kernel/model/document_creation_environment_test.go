package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"time"
	"wiki-kernel/internal/doclife"
)

func TestDocumentCreationEnvironmentOwnerGateAndOuterContext(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	tx := &Transaction{environment: &transactionEnvironment{ctx: ctx}}
	q := doclife.CreationRequest{Mode: "hpath", Box: "box", HPath: "/Created", ID: "20261027230003-new0001"}
	if _, e := tx.documentCreation(q); !errors.Is(e, doclife.ErrDependency) {
		t.Fatal(e)
	}
	check := func(c context.Context) {
		if c != ctx {
			t.Fatal("outer ctx not forwarded")
		}
	}
	e := &documentCreationEnvironment{gate: newAttributeMutationGate(), services: doclife.CreationServices{Box: func(c context.Context, _ string) (bool, error) { check(c); return true, nil }, Find: func(c context.Context, _, _, _, _ string) (doclife.Location, bool, error) {
		check(c)
		return doclife.Location{}, false, nil
	}, Create: func(c context.Context, box, p, title, dom string) (*parse.Tree, error) {
		check(c)
		return &parse.Tree{ID: q.ID, Box: box, Path: p, HPath: "/" + title, Root: &ast.Node{ID: q.ID, Type: ast.NodeDocument}}, nil
	}, Language: func(int) string { return "language" }, NewID: func() string { return q.ID }}}
	tx.environment.documentCreation = e
	tx.environment.ctx = nil
	if _, err := tx.documentCreation(q); !errors.Is(err, doclife.ErrDependency) {
		t.Fatal(err)
	}
	tx.environment.ctx = ctx
	r, err := tx.documentCreation(q)
	if err != nil || r.ID != q.ID || r.Path != "/"+q.ID+".docj" {
		t.Fatal(r, err)
	}
	<-e.gate.token
	c, finish := context.WithTimeout(ctx, 10*time.Millisecond)
	tx.environment.ctx = c
	_, err = tx.documentCreation(q)
	finish()
	e.gate.token <- struct{}{}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	tx.environment.ctx = ctx
	stop()
	if r, err = tx.documentCreation(q); !errors.Is(err, context.Canceled) || r.Tree != nil {
		t.Fatal(r, err)
	}
}
