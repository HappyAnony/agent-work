package doclife

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
)

func headingMoveFixture() *parse.Tree {
	root := &ast.Node{ID: "root", Type: ast.NodeDocument}
	for _, v := range []struct {
		id    string
		level int
	}{{"one", 1}, {"child", 2}, {"two", 1}, {"three", 1}} {
		root.AppendChild(&ast.Node{ID: v.id, Type: ast.NodeHeading, HeadingLevel: v.level})
	}
	return &parse.Tree{Root: root}
}
func TestMoveHeadingOriginalOrderLevelsHistoryAndFailure(t *testing.T) {
	for _, q := range []HeadingMoveRequest{{ID: "one", PreviousID: "two"}, {ID: "three", ParentID: "one"}, {ID: "three"}} {
		tree := headingMoveFixture()
		calls := 0
		changed, e := MoveHeading(context.Background(), tree, q, func(context.Context, *parse.Tree) error {
			calls++
			if tree.Root.FirstChild.ID != "one" {
				t.Fatal("history after mutation")
			}
			return nil
		})
		if e != nil || !changed || calls != 1 {
			t.Fatal(q, changed, calls, e)
		}
		switch {
		case q.PreviousID != "":
			if tree.Root.FirstChild.ID != "two" || tree.Root.FirstChild.Next.ID != "one" || tree.Root.FirstChild.Next.Next.ID != "child" {
				t.Fatal("wrong previous order")
			}
		case q.ParentID != "":
			if tree.Root.FirstChild.Next.ID != "three" || tree.Root.FirstChild.Next.HeadingLevel != 2 {
				t.Fatal("wrong child order")
			}
		default:
			if tree.Root.FirstChild.ID != "three" {
				t.Fatal("not first")
			}
		}
	}
	tree := headingMoveFixture()
	want := errors.New("history failure")
	if _, e := MoveHeading(context.Background(), tree, HeadingMoveRequest{ID: "one", PreviousID: "two"}, func(context.Context, *parse.Tree) error { return want }); !errors.Is(e, want) || tree.Root.FirstChild.ID != "one" {
		t.Fatal(e)
	}
	calls := 0
	before := func(context.Context, *parse.Tree) error { calls++; return nil }
	if _, e := MoveHeading(context.Background(), tree, HeadingMoveRequest{ID: "one", ParentID: "child"}, before); e == nil || calls != 0 {
		t.Fatal("moved into descendant", e, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := MoveHeading(ctx, tree, HeadingMoveRequest{ID: "one"}, before); !errors.Is(e, context.Canceled) || calls != 0 {
		t.Fatal(e, calls)
	}
}
