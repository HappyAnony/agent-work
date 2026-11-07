package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
)

func relationFixture() (*parse.Tree, map[string]*ast.Node, RelationServices) {
	nodes := map[string]*ast.Node{}
	makeNode := func(id string, typ ast.NodeType) *ast.Node {
		n := &ast.Node{ID: id, Type: typ}
		nodes[id] = n
		return n
	}
	root := makeNode("root", ast.NodeDocument)
	root.AppendChild(makeNode("before", ast.NodeHeading))
	root.AppendChild(&ast.Node{Type: ast.NodeKramdownBlockIAL})
	list := makeNode("list", ast.NodeList)
	one := makeNode("one", ast.NodeListItem)
	two := makeNode("two", ast.NodeListItem)
	one.AppendChild(&ast.Node{Type: ast.NodeTaskListItemMarker})
	p := makeNode("p", ast.NodeParagraph)
	p.AppendChild(makeNode("inline", ast.NodeText))
	one.AppendChild(p)
	one.AppendChild(&ast.Node{Type: ast.NodeKramdownBlockIAL})
	one.AppendChild(makeNode("p2", ast.NodeParagraph))
	two.AppendChild(makeNode("other", ast.NodeParagraph))
	list.AppendChild(one)
	list.AppendChild(two)
	root.AppendChild(list)
	root.AppendChild(makeNode("after", ast.NodeParagraph))
	tree := &parse.Tree{Root: root, Box: "owner", Path: "/root.docj"}
	s := RelationServices{Trees: func(_ context.Context, ids []string) (map[string]*parse.Tree, error) {
		m := map[string]*parse.Tree{}
		for _, id := range ids {
			if id != "absent" {
				m[id] = tree
			}
		}
		return m, nil
	}, Load: func(_ context.Context, id string) (*parse.Tree, error) {
		if id == "absent" {
			return nil, nil
		}
		return tree, nil
	}, BlockMissing: errors.New("missing")}
	return tree, nodes, s
}

func TestBlockRelationsMetadataNearestBlocksAndPayload(t *testing.T) {
	ctx := context.Background()
	tree, n, s := relationFixture()
	r, e := Relations(ctx, RelationRequest{Mode: "infos", IDs: []string{"list", "p", "root", "inline", "ghost", "absent", "list"}}, s)
	if e != nil || len(r.Infos) != 5 || r.Infos["list"].ParentID != "root" || r.Infos["list"].PreviousID != "before" || r.Infos["list"].PreviousType != "NodeHeading" || r.Infos["list"].NextID != "after" || r.Infos["p"].ParentID != "one" || r.Infos["p"].NextID != "p2" || r.Infos["inline"].ParentID != "p" {
		t.Fatal(r, e)
	}
	if !reflect.DeepEqual(r.Infos["ghost"], &BlockTreeInfo{ID: "ghost"}) || r.Infos["root"].Type != "NodeDocument" || r.Infos["root"].ParentID != "" {
		t.Fatal(r)
	}
	r.Infos["list"].ParentID = "changed"
	again, e := Relations(ctx, RelationRequest{Mode: "infos", IDs: []string{"list"}}, s)
	if e != nil || again.Infos["list"].ParentID != "root" || n["list"].Box != "" || n["list"].Path != "" || tree.Root.Parent != nil {
		t.Fatal("metadata aliases/mutates tree", again, e)
	}
	r, e = Relations(ctx, RelationRequest{Mode: "infos"}, s)
	if e != nil || r.Infos == nil || len(r.Infos) != 0 {
		t.Fatal(r, e)
	}
}

func TestBlockRelationsSiblingKeepsAncestorAndSelfInclusiveRules(t *testing.T) {
	ctx := context.Background()
	_, _, s := relationFixture()
	for _, tc := range []struct {
		id   string
		want RelationIDs
	}{{"list", RelationIDs{Parent: "root", Next: "after"}}, {"p", RelationIDs{Parent: "one", Next: "two"}}, {"other", RelationIDs{Parent: "two", Previous: "one", Next: "after"}}, {"after", RelationIDs{Parent: "root", Previous: "list"}}, {"root", RelationIDs{}}, {"inline", RelationIDs{}}, {"ghost", RelationIDs{}}, {"absent", RelationIDs{}}} {
		r, e := Relations(ctx, RelationRequest{Mode: "sibling", ID: tc.id}, s)
		if e != nil || r.IDs != tc.want {
			t.Fatal(tc, r, e)
		}
	}
}

func TestBlockRelationsRelevantPhysicalEdgesAndOneIALSkip(t *testing.T) {
	ctx := context.Background()
	_, nodes, s := relationFixture()
	for _, tc := range []struct {
		id   string
		want RelationIDs
	}{{"list", RelationIDs{Parent: "root", Previous: "before", Next: "after"}}, {"p", RelationIDs{Parent: "one", Next: "p2"}}, {"other", RelationIDs{Parent: "two"}}, {"inline", RelationIDs{Parent: "p"}}, {"root", RelationIDs{}}} {
		r, e := Relations(ctx, RelationRequest{Mode: "relevant", ID: tc.id}, s)
		if e != nil || r.IDs != tc.want {
			t.Fatal(tc, r, e)
		}
	}
	nodes["list"].InsertBefore(&ast.Node{Type: ast.NodeKramdownBlockIAL})
	r, e := Relations(ctx, RelationRequest{Mode: "relevant", ID: "list"}, s)
	if e != nil || r.IDs.Previous != "" {
		t.Fatal("skipped more than one IAL", r, e)
	}
	for _, id := range []string{"ghost", "absent"} {
		r, e = Relations(ctx, RelationRequest{Mode: "relevant", ID: id}, s)
		if !errors.Is(e, s.BlockMissing) || !reflect.DeepEqual(r, RelationResult{}) {
			t.Fatal(r, e)
		}
	}
}

func TestBlockRelationsCopiesRequestsAndRejectsFailures(t *testing.T) {
	ctx := context.Background()
	tree, _, s := relationFixture()
	ids := []string{"list"}
	s.Trees = func(_ context.Context, got []string) (map[string]*parse.Tree, error) {
		got[0] = "changed"
		return map[string]*parse.Tree{"list": tree}, nil
	}
	if _, e := Relations(ctx, RelationRequest{Mode: "infos", IDs: ids}, s); e != nil || ids[0] != "list" {
		t.Fatal(ids, e)
	}
	boom := errors.New("read")
	s.Trees = func(context.Context, []string) (map[string]*parse.Tree, error) { return nil, boom }
	if r, e := Relations(ctx, RelationRequest{Mode: "infos", IDs: []string{"list"}}, s); !errors.Is(e, boom) || !reflect.DeepEqual(r, RelationResult{}) {
		t.Fatal(r, e)
	}
	s.Load = func(context.Context, string) (*parse.Tree, error) { return nil, boom }
	if r, e := Relations(ctx, RelationRequest{Mode: "sibling"}, s); !errors.Is(e, boom) || !reflect.DeepEqual(r, RelationResult{}) {
		t.Fatal(r, e)
	}
	s.Trees = func(context.Context, []string) (map[string]*parse.Tree, error) {
		return map[string]*parse.Tree{"broken": {Root: nil}}, nil
	}
	if r, e := Relations(ctx, RelationRequest{Mode: "infos", IDs: []string{"broken"}}, s); e == nil || !reflect.DeepEqual(r, RelationResult{}) {
		t.Fatal(r, e)
	}
	if _, e := Relations(ctx, RelationRequest{Mode: "infos"}, RelationServices{}); !errors.Is(e, ErrRelationDependency) {
		t.Fatal(e)
	}
}

func TestBlockRelationsCancellationBeforeAndAfterActualLoad(t *testing.T) {
	_, _, s := relationFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r, e := Relations(ctx, RelationRequest{Mode: "infos"}, s); !errors.Is(e, context.Canceled) || !reflect.DeepEqual(r, RelationResult{}) {
		t.Fatal(r, e)
	}
	for _, mode := range []string{"infos", "sibling", "relevant"} {
		ctx, cancel := context.WithCancel(context.Background())
		tree, _, s := relationFixture()
		s.Load = func(context.Context, string) (*parse.Tree, error) { cancel(); return tree, nil }
		s.Trees = func(context.Context, []string) (map[string]*parse.Tree, error) {
			cancel()
			return map[string]*parse.Tree{"list": tree}, nil
		}
		if r, e := Relations(ctx, RelationRequest{Mode: mode, ID: "list", IDs: []string{"list"}}, s); !errors.Is(e, context.Canceled) || !reflect.DeepEqual(r, RelationResult{}) {
			t.Fatal(mode, r, e)
		}
	}
}
