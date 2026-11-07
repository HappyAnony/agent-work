package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"strconv"
	"testing"
)

func foldFixture() (*parse.Tree, map[string]*ast.Node, FoldServices) {
	nodes := map[string]*ast.Node{}
	makeNode := func(id string, typ ast.NodeType, level int, folded bool) *ast.Node {
		n := &ast.Node{ID: id, Type: typ, HeadingLevel: level}
		if folded {
			n.SetIALAttr("fold", "1")
		}
		nodes[id] = n
		return n
	}
	root := makeNode("root", ast.NodeDocument, 0, false)
	root.AppendChild(makeNode("h1", ast.NodeHeading, 1, true))
	root.AppendChild(makeNode("h2", ast.NodeHeading, 2, true))
	p := makeNode("p", ast.NodeParagraph, 0, false)
	p.AppendChild(makeNode("inline", ast.NodeText, 0, false))
	root.AppendChild(p)
	root.AppendChild(makeNode("peer", ast.NodeHeading, 2, false))
	root.AppendChild(makeNode("after", ast.NodeParagraph, 0, false))
	box := makeNode("box", ast.NodeBlockquote, 0, true)
	box.AppendChild(makeNode("inner", ast.NodeParagraph, 0, false))
	root.AppendChild(box)
	tree := &parse.Tree{Root: root, Box: "owner", Path: "/root.docj"}
	s := FoldServices{Load: func(_ context.Context, id string) (*parse.Tree, error) {
		if nodes[id] == nil {
			return nil, nil
		}
		return tree, nil
	}}
	return tree, nodes, s
}

func TestBlockFoldOriginalHeadingAndPhysicalRules(t *testing.T) {
	_, n, s := foldFixture()
	for _, tc := range []struct {
		id, parent   string
		folded, root bool
	}{
		{"p", "h1", true, false}, {"h2", "h1", true, false}, {"h1", "h1", true, false},
		// Unfolded parent stops at peer, while state keeps walking to h1.
		{"after", "after", true, false}, {"peer", "h1", true, false},
		{"inner", "box", true, false}, {"root", "root", false, true},
		{"inline", "", true, false}, {"missing", "", false, false},
	} {
		u, e := Fold(context.Background(), FoldRequest{"unfolded-parent", tc.id}, s)
		if e != nil || u.ParentID != tc.parent || u.IsFolded || u.IsRoot {
			t.Fatal(tc, u, e)
		}
		r, e := Fold(context.Background(), FoldRequest{"state", tc.id}, s)
		if e != nil || r.IsFolded != tc.folded || r.IsRoot != tc.root || r.ParentID != "" {
			t.Fatal(tc, r, e)
		}
	}
	// A target's own fold is not an ancestor for unfolded-parent; root fold
	// is excluded there, but included in the state parent chain.
	n["h1"].SetIALAttr("fold", "0")
	n["h2"].SetIALAttr("fold", "0")
	n["p"].SetIALAttr("fold", "1")
	u, e := Fold(context.Background(), FoldRequest{"unfolded-parent", "p"}, s)
	if e != nil || u.ParentID != "p" {
		t.Fatal(u, e)
	}
	n["p"].SetIALAttr("fold", "0")
	n["root"].SetIALAttr("fold", "1")
	r, e := Fold(context.Background(), FoldRequest{"state", "after"}, s)
	if e != nil || !r.IsFolded || r.IsRoot {
		t.Fatal(r, e)
	}
	r, e = Fold(context.Background(), FoldRequest{"state", "root"}, s)
	if e != nil || !r.IsFolded || !r.IsRoot {
		t.Fatal(r, e)
	}
}

func TestBlockFoldStateThirtyTwoReadsAndBoundaries(t *testing.T) {
	for _, foldAt := range []int{31, 32, 33} {
		root := &ast.Node{ID: "root", Type: ast.NodeDocument}
		parent := root
		for i := 33; i >= 0; i-- {
			n := &ast.Node{ID: strconv.Itoa(i), Type: ast.NodeBlockquote}
			if i == foldAt {
				n.SetIALAttr("fold", "1")
			}
			parent.AppendChild(n)
			parent = n
		}
		tree := &parse.Tree{Root: root}
		calls := 0
		s := FoldServices{Load: func(context.Context, string) (*parse.Tree, error) { calls++; return tree, nil }}
		r, e := Fold(context.Background(), FoldRequest{"state", "0"}, s)
		if e != nil || r.IsFolded != (foldAt == 31) || calls != 33 {
			t.Fatal(foldAt, r, e, calls)
		}
	}
}

func TestBlockFoldMissingAnonymousAndReadOnly(t *testing.T) {
	tree, n, s := foldFixture()
	n["p"].Box = "original"
	n["p"].Path = "original"
	for _, mode := range []string{"state", "unfolded-parent"} {
		if _, e := Fold(context.Background(), FoldRequest{mode, "p"}, s); e != nil {
			t.Fatal(e)
		}
	}
	if n["p"].Box != "original" || n["p"].Path != "original" {
		t.Fatal("read annotated AST")
	}
	// Even a loader that could return a tree for an empty ID must not be
	// called after root: the old getBlock returned immediately in that case.
	oldLoad := s.Load
	s.Load = func(c context.Context, id string) (*parse.Tree, error) {
		if id == "" {
			t.Fatal("empty parent loaded")
		}
		return oldLoad(c, id)
	}
	if r, e := Fold(context.Background(), FoldRequest{"state", "root"}, s); e != nil || r.IsFolded || !r.IsRoot {
		t.Fatal(r, e)
	}
	// Loaded tree with a missing node differs from a missing tree only in
	// isRoot detection; both ordinary reads return empty/false.
	s.Load = func(context.Context, string) (*parse.Tree, error) { return tree, nil }
	for _, mode := range []string{"state", "unfolded-parent"} {
		r, e := Fold(context.Background(), FoldRequest{mode, "ghost"}, s)
		if e != nil || r != (FoldResult{}) {
			t.Fatal(r, e)
		}
	}
	// A folded anonymous physical container followed by an unfolded parent
	// preserves the original empty-ID early return.
	n["h1"].SetIALAttr("fold", "0")
	n["box"].ID = ""
	r, e := Fold(context.Background(), FoldRequest{"unfolded-parent", "inner"}, s)
	if e != nil || r.ParentID != "" {
		t.Fatal(r, e)
	}
}

func TestBlockFoldDependencyLoadAndLateFailure(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("load")
	for _, mode := range []string{"state", "unfolded-parent"} {
		if _, e := Fold(ctx, FoldRequest{mode, "root"}, FoldServices{}); !errors.Is(e, ErrFoldDependency) {
			t.Fatal(e)
		}
		if _, e := Fold(nil, FoldRequest{mode, "root"}, FoldServices{Load: func(context.Context, string) (*parse.Tree, error) { return nil, nil }}); !errors.Is(e, ErrFoldDependency) {
			t.Fatal(e)
		}
		r, e := Fold(ctx, FoldRequest{mode, "root"}, FoldServices{Load: func(context.Context, string) (*parse.Tree, error) { return nil, sentinel }})
		if !errors.Is(e, sentinel) || r != (FoldResult{}) {
			t.Fatal(r, e)
		}
		r, e = Fold(ctx, FoldRequest{mode, "root"}, FoldServices{Load: func(context.Context, string) (*parse.Tree, error) { return &parse.Tree{}, nil }})
		if e == nil || r != (FoldResult{}) {
			t.Fatal(r, e)
		}
	}
	tree, _, s := foldFixture()
	calls := 0
	s.Load = func(context.Context, string) (*parse.Tree, error) {
		calls++
		if calls > 1 {
			return nil, sentinel
		}
		return tree, nil
	}
	r, e := Fold(ctx, FoldRequest{"state", "root"}, s)
	if !errors.Is(e, sentinel) || !reflect.DeepEqual(r, FoldResult{}) {
		t.Fatal("partial root success", r, e)
	}
	if _, e = Fold(ctx, FoldRequest{"unknown", "root"}, s); e == nil {
		t.Fatal("bad mode")
	}
}

func TestBlockFoldCancellationBeforeAndAfterLoad(t *testing.T) {
	for _, mode := range []string{"state", "unfolded-parent"} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r, e := Fold(ctx, FoldRequest{mode, "root"}, FoldServices{Load: func(context.Context, string) (*parse.Tree, error) {
			t.Fatal("cancelled loader called")
			return nil, nil
		}})
		if !errors.Is(e, context.Canceled) || r != (FoldResult{}) {
			t.Fatal(r, e)
		}
		ctx, cancel = context.WithCancel(context.Background())
		tree, _, s := foldFixture()
		s.Load = func(actual context.Context, _ string) (*parse.Tree, error) {
			if actual != ctx {
				t.Fatal("wrong context")
			}
			cancel()
			return tree, nil
		}
		r, e = Fold(ctx, FoldRequest{mode, "root"}, s)
		if !errors.Is(e, context.Canceled) || r != (FoldResult{}) {
			t.Fatal(r, e)
		}
	}
}
