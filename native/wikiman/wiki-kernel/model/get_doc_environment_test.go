package model

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/av"
	"wiki-kernel/internal/nativeparser"
)

func getDocFixture(owner string) (*parse.Tree, *getDocEnvironment, *[]recentGetDocSnapshot) {
	root := &ast.Node{Type: ast.NodeDocument, ID: "20261027100000-doc0001"}
	root.SetIALAttr("title", owner)
	root.SetIALAttr("id", root.ID)
	tree := &parse.Tree{ID: root.ID, Root: root, Box: owner, Path: "/" + root.ID + ".docj", HPath: "/" + owner}
	for i := 0; i < 6; i++ {
		n := &ast.Node{Type: ast.NodeParagraph, ID: fmt.Sprintf("2026102710000%d-par0001", i)}
		n.SetIALAttr("id", n.ID)
		n.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(owner + " needle " + fmt.Sprint(i))})
		root.AppendChild(n)
	}
	root.FirstChild.SetIALAttr(av.NodeAttrNameAvs, "database")
	parser := nativeparser.NewEditor(nativeparser.EditorOptions{})
	recent := []recentGetDocSnapshot{}
	env := &getDocEnvironment{ctx: context.Background(), dynamicLoadBlocks: 2, blockMissing: errors.New("missing-block"), treeMissing: errors.New("missing-tree"),
		wait: func(context.Context) error { return nil }, load: func(context.Context, string) (*parse.Tree, error) { return tree, nil }, parser: func(context.Context) (*lute.Lute, error) { return parser, nil },
		backlink: func(_ context.Context, n *ast.Node, refs map[string]string) ([]*ast.Node, bool, error) {
			nodes, expand := getBacklinkRenderNodes(n, refs)
			return nodes, expand, nil
		},
		references: func(context.Context, string) (map[string]int, error) {
			return map[string]int{root.FirstChild.ID: 3}, nil
		}, virtualKeywords: func(context.Context, *ast.Node) ([]string, error) { return nil, nil },
		keywords: func(context.Context, string, map[string]bool, int, string) ([]string, error) {
			return []string{"needle"}, nil
		}, avNames: func(context.Context, string) (string, error) { return owner + " database", nil },
		virtual: func(context.Context, *ast.Node, *[]*ast.Node, []string, map[string]int, *lute.Lute) (bool, error) {
			return false, nil
		},
		render: func(_ context.Context, t *parse.Tree, p *lute.Lute) (string, error) {
			return p.Tree2BlockDOM(t, p.RenderOptions), nil
		}, recent: func(_ context.Context, s recentGetDocSnapshot) error { recent = append(recent, s); return nil }}
	return tree, env, &recent
}

func getDocTreeState(tree *parse.Tree) map[*ast.Node]ast.Node {
	out := map[*ast.Node]ast.Node{}
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if enter {
			c := *n
			c.Tokens = slices.Clone(n.Tokens)
			c.KramdownIAL = make([][]string, len(n.KramdownIAL))
			if n.KramdownIAL == nil {
				c.KramdownIAL = nil
			}
			for i, v := range n.KramdownIAL {
				c.KramdownIAL[i] = slices.Clone(v)
			}
			out[n] = c
		}
		return ast.WalkContinue
	})
	return out
}

func TestGetDocContextOwnersRestoreAndRender(t *testing.T) {
	for _, owner := range []string{"owner-one", "owner-two"} {
		t.Run(owner, func(t *testing.T) {
			tree, e, recent := getDocFixture(owner)
			before := getDocTreeState(tree)
			p, _ := e.parser(e.ctx)
			p.RenderOptions.NodeIndexStart = 79
			q := getDocRequest{ID: tree.Root.ID, Mode: 0, Size: 100, Query: "needle", Highlight: true, QueryTypes: map[string]bool{"text": true}}
			for i := 0; i < 2; i++ {
				r, err := getDocumentWithEnvironment(q, e)
				if err != nil || !r.Scroll || r.RootID != tree.ID || r.Box != owner || !strings.Contains(r.DOM, owner+" database") || !strings.Contains(r.DOM, "search-mark") || !strings.Contains(r.DOM, "refcount=\"3\"") {
					t.Fatalf("%+v %v", r, err)
				}
				if !reflect.DeepEqual(before, getDocTreeState(tree)) {
					t.Fatal("render changed owner AST")
				}
			}
			if p.RenderOptions.NodeIndexStart != 79 || len(*recent) != 2 || (*recent)[0].Title != owner || (*recent)[0].Box != owner {
				t.Fatal("owner metadata/options leaked", recent)
			}
			if !q.QueryTypes["text"] {
				t.Fatal("request changed")
			}
		})
	}
}

func TestGetDocDynamicModesRangeAndFolds(t *testing.T) {
	tree, e, _ := getDocFixture("modes")
	first := tree.Root.FirstChild
	middle := first.Next.Next
	for _, tc := range []struct {
		mode int
		ids  []string
		eof  bool
	}{{0, []string{middle.ID}, false}, {1, []string{first.ID, first.Next.ID}, false}, {2, []string{middle.Next.ID, middle.Next.Next.ID, middle.Next.Next.Next.ID}, false}, {3, []string{first.ID, first.Next.ID, middle.ID, middle.Next.ID, middle.Next.Next.ID, middle.Next.Next.Next.ID}, false}, {4, []string{first.ID, first.Next.ID, middle.ID}, true}} {
		nodes, eof, err := loadNodesByModeWithContext(e.ctx, middle, 0, tc.mode, 100, false, false)
		ids := []string{}
		for _, n := range nodes {
			ids = append(ids, n.ID)
		}
		if err != nil || !reflect.DeepEqual(ids, tc.ids) || eof != tc.eof {
			t.Fatalf("mode %d: %v %v %v", tc.mode, ids, eof, err)
		}
	}
	nodes, eof, err := loadNodesByStartEndWithContext(e.ctx, tree, first.Next.ID, middle.Next.ID, 100)
	if err != nil || len(nodes) != 3 || eof {
		t.Fatal(nodes, eof, err)
	}
	nodes, _, err = loadNodesByStartEndWithContext(e.ctx, tree, first.ID, tree.Root.LastChild.ID, 2)
	if err != nil || len(nodes) != 2 {
		t.Fatal(nodes, err)
	}
	first.Type = ast.NodeHeading
	first.HeadingLevel = 1
	first.SetIALAttr("fold", "1")
	first.Next.SetIALAttr("heading-fold", "1")
	middle.Type = ast.NodeHeading
	middle.HeadingLevel = 1
	nodes, _, err = loadNodesByModeWithContext(e.ctx, first, 0, 2, 100, false, true)
	if err != nil || len(nodes) != 4 || nodes[0] != middle {
		t.Fatal("fold boundary", nodes, err)
	}
	for mode := 0; mode < 5; mode++ {
		r, err := getDocumentWithEnvironment(getDocRequest{ID: tree.ID, Mode: mode, Size: 3}, e)
		if err != nil || r.RootID != tree.ID {
			t.Fatal(mode, r, err)
		}
	}
	r, err := getDocumentWithEnvironment(getDocRequest{ID: tree.ID, StartID: middle.ID, EndID: middle.Next.ID, Size: 100}, e)
	if err != nil || !strings.Contains(r.DOM, middle.ID) || strings.Contains(r.DOM, first.ID) {
		t.Fatal(r, err)
	}
	r, err = getDocumentWithEnvironment(getDocRequest{ID: middle.ID, Backlink: true, OriginalRefs: map[string]string{middle.ID: "original"}, Size: 10}, e)
	if err != nil || !strings.Contains(r.DOM, middle.ID) {
		t.Fatal(r, err)
	}
}

func TestGetDocContextFailureCancelAndPanicRestore(t *testing.T) {
	for _, stage := range []string{"wait", "load", "parser", "backlink", "references", "virtualKeywords", "keywords", "avNames", "virtual", "render", "recent", "panic", "cancel-render"} {
		t.Run(stage, func(t *testing.T) {
			tree, e, recent := getDocFixture(stage)
			before := getDocTreeState(tree)
			boom := errors.New(stage)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.ctx = ctx
			switch stage {
			case "wait":
				e.wait = func(context.Context) error { return boom }
			case "load":
				e.load = func(context.Context, string) (*parse.Tree, error) { return nil, boom }
			case "parser":
				e.parser = func(context.Context) (*lute.Lute, error) { return nil, boom }
			case "backlink":
				e.backlink = func(context.Context, *ast.Node, map[string]string) ([]*ast.Node, bool, error) {
					return nil, false, boom
				}
			case "references":
				e.references = func(context.Context, string) (map[string]int, error) { return nil, boom }
			case "virtualKeywords":
				e.virtualKeywords = func(context.Context, *ast.Node) ([]string, error) { return nil, boom }
			case "keywords":
				e.keywords = func(context.Context, string, map[string]bool, int, string) ([]string, error) { return nil, boom }
			case "avNames":
				e.avNames = func(context.Context, string) (string, error) { return "", boom }
			case "virtual":
				e.virtual = func(context.Context, *ast.Node, *[]*ast.Node, []string, map[string]int, *lute.Lute) (bool, error) {
					return false, boom
				}
			case "render":
				e.render = func(context.Context, *parse.Tree, *lute.Lute) (string, error) { return "partial", boom }
			case "recent":
				e.recent = func(context.Context, recentGetDocSnapshot) error { return boom }
			case "panic":
				e.render = func(context.Context, *parse.Tree, *lute.Lute) (string, error) { panic("renderer") }
			case "cancel-render":
				e.render = func(context.Context, *parse.Tree, *lute.Lute) (string, error) { cancel(); return "partial", nil }
			}
			r, err := getDocumentWithEnvironment(getDocRequest{ID: tree.ID, Size: 100, Query: "needle", Highlight: true, Backlink: stage == "backlink"}, e)
			if err == nil || !reflect.DeepEqual(r, getDocResult{}) || !reflect.DeepEqual(before, getDocTreeState(tree)) || len(*recent) != 0 {
				t.Fatal("partial success or mutation", r, err)
			}
		})
	}
}

func TestGetDocContextNoFallbackOuterContextAndMissing(t *testing.T) {
	tx := &Transaction{environment: &transactionEnvironment{ctx: context.Background()}}
	if _, err := tx.getDocument(getDocRequest{}); err == nil {
		t.Fatal("implicit global fallback")
	}
	tree, e, _ := getDocFixture("outer")
	ctx, cancel := context.WithCancel(context.Background())
	tx.environment.ctx = ctx
	tx.environment.getDoc = e
	load := e.load
	e.load = func(actual context.Context, id string) (*parse.Tree, error) {
		if actual != ctx {
			t.Fatal("loader did not receive outer context")
		}
		return load(actual, id)
	}
	render := e.render
	e.render = func(actual context.Context, tree *parse.Tree, parser *lute.Lute) (string, error) {
		if actual != ctx {
			t.Fatal("renderer did not receive outer context")
		}
		return render(actual, tree, parser)
	}
	if _, err := tx.getDocument(getDocRequest{ID: tree.ID, Size: 100}); err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err := tx.getDocument(getDocRequest{ID: tree.ID}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, e, _ = getDocFixture("missing")
	e.load = func(context.Context, string) (*parse.Tree, error) { return nil, e.blockMissing }
	if _, err := getDocumentWithEnvironment(getDocRequest{Mode: 0}, e); err != e.treeMissing {
		t.Fatal(err)
	}
	if _, err := getDocumentWithEnvironment(getDocRequest{Mode: 2}, e); err != e.blockMissing {
		t.Fatal(err)
	}
	e.render = nil
	if _, err := getDocumentWithEnvironment(getDocRequest{}, e); err == nil {
		t.Fatal("missing renderer accepted")
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, _, err := loadNodesByModeWithContext(ctx, tree.Root.FirstChild, 0, 0, 1, true, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, _, err := loadNodesByStartEndWithContext(ctx, tree, "a", "b", 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestGetDocContextOriginalRenderModesAndInputCopies(t *testing.T) {
	for mode := 0; mode < 5; mode++ {
		for _, index := range []int{0, 2, 999} {
			tree, native, _ := getDocFixture("original")
			_, original, _ := getDocFixture("original")
			original.legacy = true
			q := getDocRequest{ID: tree.ID, Index: index, Mode: mode, Size: 2, Query: "needle", Highlight: true}
			a, err := getDocumentWithEnvironment(q, original)
			b, nativeErr := getDocumentWithEnvironment(q, native)
			if err != nil || nativeErr != nil || !reflect.DeepEqual(a, b) {
				t.Fatalf("mode=%d index=%d: original=%+v native=%+v errors=%v/%v", mode, index, a, b, err, nativeErr)
			}
		}
	}
	tree, e, _ := getDocFixture("copies")
	keys := []string{"#needle#"}
	e.keywords = func(_ context.Context, _ string, types map[string]bool, _ int, _ string) ([]string, error) {
		delete(types, "text")
		return keys, nil
	}
	q := getDocRequest{ID: tree.ID, Query: "needle", QueryTypes: map[string]bool{"text": true}, Size: 100}
	r, err := getDocumentWithEnvironment(q, e)
	if err != nil || !q.QueryTypes["text"] || keys[0] != "#needle#" || r.Keywords[0] != "needle" {
		t.Fatal(r, err, q, keys)
	}
	r.Keywords[0] = "changed"
	if keys[0] != "#needle#" {
		t.Fatal("result aliases service keywords")
	}
	e.caseSensitive = true
	e.keywords = func(context.Context, string, map[string]bool, int, string) ([]string, error) {
		return []string{"NEEDLE"}, nil
	}
	q.Highlight = true
	r, err = getDocumentWithEnvironment(q, e)
	if err != nil || strings.Contains(r.DOM, "search-mark") {
		t.Fatal("case-sensitive highlight", r, err)
	}
	e.caseSensitive = false
	r, err = getDocumentWithEnvironment(q, e)
	if err != nil || !strings.Contains(r.DOM, "search-mark") {
		t.Fatal("owner case-insensitive highlight", r, err)
	}
	q.QueryMethod = 2
	r, err = getDocumentWithEnvironment(q, e)
	if err != nil || len(r.Keywords) != 0 || strings.Contains(r.DOM, "search-mark") {
		t.Fatal("SQL query must not highlight", r, err)
	}
}

func TestGetDocSnapshotRestoresNestedRenderMetadata(t *testing.T) {
	tree, e, _ := getDocFixture("nested")
	n := tree.Root.FirstChild
	n.ListData = &ast.ListData{Num: 2, Marker: []byte("-")}
	list := n.ListData
	n.CodeBlockInfo = []byte("go")
	n.TableAligns = []int{1, 2}
	tree.Marks = []string{"a"}
	e.render = func(_ context.Context, _ *parse.Tree, _ *lute.Lute) (string, error) {
		list.Num = 9
		list.Marker[0] = '*'
		n.CodeBlockInfo[0] = 'x'
		n.TableAligns[0] = 3
		tree.Marks[0] = "changed"
		return "", errors.New("render-failed")
	}
	if _, err := getDocumentWithEnvironment(getDocRequest{ID: tree.ID, Size: 100}, e); err == nil {
		t.Fatal("error lost")
	}
	if n.ListData != list || list.Num != 2 || string(list.Marker) != "-" || string(n.CodeBlockInfo) != "go" || n.TableAligns[0] != 1 || tree.Marks[0] != "a" {
		t.Fatal("nested metadata leaked", n, tree.Marks)
	}
}
