//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/av"
)

func TestAVTransactionReadThreeModesUseCapturedOwner(t *testing.T) {
	tx, a, _, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	if got, e := tx.readAttributeView(a.ID); e != nil || got == nil || got.Name != a.Name {
		t.Fatalf("bound source read: %v %v", got, e)
	}
	old := Conf
	Conf = &AppConf{}
	defer func() { Conf = old }()
	if _, e := tx.readAttributeView(a.ID); e == nil {
		t.Fatal("source recaptured replacement")
	}
	if _, e := tx.attributeViewPrimary(a.ID, "", 1, 10); e == nil {
		t.Fatal("primary recaptured replacement")
	}
	if _, _, e := tx.attributeViewRules(a.ID, ""); e == nil {
		t.Fatal("rules recaptured replacement")
	}
}

func TestAVTransactionReadExistenceAndRuleErrorsPropagate(t *testing.T) {
	tx, a, _, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	e := tx.attributeViewReadEnvironment()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.exists(ctx, "id"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
	want := errors.New("node lookup failed")
	primary := *e
	primary.mirrors = func(string) ([]string, error) { return nil, want }
	if result, err := attributeViewPrimaryWithEnvironment(a.ID, "", 1, 10, &primary); !errors.Is(err, want) || result.name != "" || result.values != nil {
		t.Fatalf("partial primary escaped: %+v %v", result, err)
	}
	e.node = func(string) (*ast.Node, *parse.Tree, error) { return nil, nil, want }
	if _, err := selectAttributeViewReadView(a, "block", e); !errors.Is(err, want) {
		t.Fatalf("rule failure hidden: %v", err)
	}
	e.node = func(string) (*ast.Node, *parse.Tree, error) { return nil, nil, ErrBlockNotFound }
	if view, err := selectAttributeViewReadView(a, "missing", e); err != nil || view == nil {
		t.Fatalf("missing block fallback lost: %v", err)
	}
	if _, err := tx.environment.commitContext.r.registry.DB.Exec("DROP TABLE blocktrees"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.exists(context.Background(), "id"); err == nil {
		t.Fatal("existence SQL failure hidden")
	}
}
