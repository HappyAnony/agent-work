package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

func TestOriginalDeleteContextActualSourceCandidatesAndFailures(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); e != nil {
		t.Fatal(e)
	}
	const pageA = "1760000000000000002"
	const pageB = "1760000000000000003"
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"
	const block = "20261019090002-par0001"
	const ref = "20261019090004-ref0001"
	if e := os.MkdirAll(filepath.Join(root, "pages", pageB, "data"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.CopyFS(filepath.Join(root, "pages", pageB, "data"), os.DirFS(filepath.Join(root, "pages", pageA, "data"))); e != nil {
		t.Fatal(e)
	}
	m := workspace.NewManager(workspace.Options{})
	store, e := m.OpenStore(ctx, root)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Drain(ctx)
	for _, page := range []string{pageA, pageB} {
		lease, e := m.Acquire(ctx, store, page)
		if e != nil {
			t.Fatal(e)
		}
		before, e := lease.ReadDocument(ctx, box, doc)
		if e != nil {
			t.Fatal(e)
		}
		for _, id := range []string{block, ref} {
			for _, failure := range []string{"", "load", "references", "registry", "stage", "av", "finish", "missing", "cancel"} {
				t.Run(page+"/"+id+"/"+failure, func(t *testing.T) {
					callCtx, cancel := context.WithCancel(ctx)
					defer cancel()
					var candidate *parse.Tree
					staged, finished, deleted, avCalls, refCalls := 0, 0, 0, 0, 0
					want := errors.New("synthetic " + failure)
					env := &transactionEnvironment{ctx: callCtx, parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) }, lookup: func(key string) *treenode.BlockTree {
						if failure == "missing" {
							return nil
						}
						return &treenode.BlockTree{ID: key, RootID: doc, BoxID: box, Path: "/" + doc + ".docj"}
					}, load: func(_, p string, engine *lute.Lute) (*parse.Tree, error) {
						if failure == "load" {
							return nil, want
						}
						raw, e := lease.ReadDocument(ctx, box, doc)
						if e != nil {
							return nil, e
						}
						tree, e := dataparser.ParseJSONWithoutFix(raw, engine.ParseOptions)
						if e != nil {
							return nil, e
						}
						tree.ID, tree.Box, tree.Path = doc, box, p
						return tree, nil
					}, cacheIAL: func(string, map[string]string) {}, stage: func(tree *parse.Tree) error {
						staged++
						candidate = tree
						if failure == "stage" {
							return want
						}
						return callCtx.Err()
					}, finish: func(*Transaction) error {
						finished++
						if failure == "finish" {
							return want
						}
						return nil
					}, deleteReferences: func(_ context.Context, ids ...string) error {
						refCalls++
						if id == ref && (len(ids) != 1 || ids[0] != block) {
							t.Fatal(ids)
						}
						if failure == "references" {
							return want
						}
						if failure == "cancel" {
							cancel()
						}
						return callCtx.Err()
					}, deleteBlockTree: func(_ context.Context, key string) error {
						deleted++
						if key != id {
							t.Fatal(key)
						}
						if failure == "registry" {
							return want
						}
						return callCtx.Err()
					}, deleteAV: func(_ context.Context, n *ast.Node, tree *parse.Tree, tx *Transaction) error {
						avCalls++
						if n.ID != id || n.Parent != nil || tree.ID != doc {
							t.Fatal("delete effect order")
						}
						if failure == "av" {
							return want
						}
						return callCtx.Err()
					}}
					tx := &Transaction{environment: env, DoOperations: []*Operation{{Action: "delete", ID: id}}}
					result := performTx(tx)
					if failure == "" {
						if result != nil || staged != 1 || deleted != 1 || avCalls != 1 || refCalls != 1 || finished != 1 || tx.state.Load() != 2 || treenode.GetNodeInTree(candidate, id) != nil {
							t.Fatal(result, staged, deleted, avCalls, refCalls, finished, tx.state.Load())
						}
					} else if failure == "missing" {
						if result != nil || staged != 0 || finished != 1 {
							t.Fatal("missing-after-move no-op changed", result, staged, finished)
						}
					} else if result == nil || tx.state.Load() != 3 {
						t.Fatal("failed deletion accepted", result, tx.state.Load())
					}
					if !tx.m.TryLock() {
						t.Fatal("delete retained lock")
					}
					tx.m.Unlock()
					after, e := lease.ReadDocument(ctx, box, doc)
					if e != nil || string(after) != string(before) {
						t.Fatal("candidate changed source", e)
					}
				})
			}
		}
		lease.Release()
	}
}

func TestOriginalDeleteContextListPlaceholderAndIAL(t *testing.T) {
	root := &ast.Node{ID: "root", Type: ast.NodeDocument}
	list := &ast.Node{ID: "list", Type: ast.NodeList, ListData: &ast.ListData{Typ: 0}}
	item := &ast.Node{ID: "item", Type: ast.NodeListItem, ListData: &ast.ListData{Typ: 0}}
	p := &ast.Node{ID: "paragraph", Type: ast.NodeParagraph}
	root.AppendChild(list)
	list.AppendChild(item)
	item.AppendChild(p)
	item.AppendChild(&ast.Node{Type: ast.NodeKramdownBlockIAL, Tokens: []byte(`{: id="paragraph"}`)})
	tree := &parse.Tree{ID: "root", Root: root}
	env := &transactionEnvironment{ctx: context.Background(), parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) }, lookup: func(id string) *treenode.BlockTree { return &treenode.BlockTree{ID: id, RootID: "root"} }, load: func(string, string, *lute.Lute) (*parse.Tree, error) { return tree, nil }, stage: func(*parse.Tree) error { return nil }, cacheIAL: func(string, map[string]string) {}, finish: func(*Transaction) error { return nil }, deleteReferences: func(context.Context, ...string) error { return nil }, deleteBlockTree: func(context.Context, string) error { return nil }, deleteAV: func(context.Context, *ast.Node, *parse.Tree, *Transaction) error { return nil }}
	if e := performTx(&Transaction{environment: env, DoOperations: []*Operation{{Action: "delete", ID: "paragraph"}}}); e != nil {
		t.Fatal(e)
	}
	if item.FirstChild == nil || item.FirstChild.Type != ast.NodeParagraph || item.FirstChild.ID == "paragraph" || item.FirstChild.Next != nil {
		t.Fatal("empty list placeholder/IAL rules changed")
	}
	env.deleteAV = nil
	if env.validate([]*Operation{{Action: "delete"}}) == nil {
		t.Fatal("unbound AV side effect allowed")
	}
}
