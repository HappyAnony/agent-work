package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/doclife"
)

func TestDocumentLifecycleEnvironmentOuterContextAndNoFallback(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	tx := &Transaction{environment: &transactionEnvironment{ctx: ctx}}
	if _, e := tx.documentLifecycle(doclife.Request{Action: "rename"}); !errors.Is(e, doclife.ErrDependency) {
		t.Fatal(e)
	}
	check := func(c context.Context) {
		if c != ctx {
			t.Fatal("outer context missing")
		}
	}
	root := &ast.Node{ID: "20261027230000-root001", Type: ast.NodeDocument}
	root.SetIALAttr("title", "old")
	called := false
	tx.environment.documentLifecycle = &documentLifecycleEnvironment{services: doclife.Services{Box: func(c context.Context, _ string) (bool, error) { check(c); return true, nil }, Load: func(c context.Context, _, _ string) (*parse.Tree, error) {
		check(c)
		return &parse.Tree{Root: root, ID: root.ID, Box: "box", Path: "/" + root.ID + ".docj", HPath: "/old"}, nil
	}, Lookup: func(context.Context, string) (doclife.Location, bool, error) { return doclife.Location{}, false, nil }, Children: func(c context.Context, _, _ string) ([]doclife.Location, error) { check(c); return nil, nil }, Language: func(int) string { return "language" }, Now: func() string { return "20261027235959" }, Apply: func(c context.Context, p *doclife.Plan) error {
		check(c)
		called = p.Tree.Root.IALAttr("title") == "new"
		return nil
	}}}
	q := doclife.Request{Action: "rename", Box: "box", Path: "/" + root.ID + ".docj", Title: "new"}
	if _, e := tx.documentLifecycle(q); e != nil || !called {
		t.Fatal(e, called)
	}
	stop()
	called = false
	if _, e := tx.documentLifecycle(q); !errors.Is(e, context.Canceled) || called {
		t.Fatal(e, called)
	}
}
