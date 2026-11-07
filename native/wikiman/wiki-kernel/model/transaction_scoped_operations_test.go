package model

import (
	"context"
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

func TestOriginalScopedSetAttrsAndFoldHeadingSameIDs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	const pageA = "1760000000000000002"
	const pageB = "1760000000000000003"
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"
	const id = "20261019090002-par0001"
	bData := filepath.Join(root, "pages", pageB, "data")
	if err := os.MkdirAll(bData, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(bData, os.DirFS(filepath.Join(root, "pages", pageA, "data"))); err != nil {
		t.Fatal(err)
	}
	m := workspace.NewManager(workspace.Options{})
	store, err := m.OpenStore(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Drain(ctx)
	for _, page := range []string{pageA, pageB} {
		l, err := m.Acquire(ctx, store, page)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := l.ReadDocument(ctx, box, doc)
		if err != nil {
			t.Fatal(err)
		}
		engine := nativeparser.NewLute(nativeparser.Markdown{})
		tree, _, err := dataparser.ParseJSON(raw, engine.ParseOptions)
		if err != nil {
			t.Fatal(err)
		}
		tree.ID, tree.Box, tree.Path = doc, box, "/"+doc+".docj"
		tree.Root.FirstChild.Type = ast.NodeHeading
		tree.Root.FirstChild.HeadingLevel = 1
		cached := map[string]map[string]string{}
		stages, indexes, syncs := 0, 0, 0
		e := &transactionEnvironment{ctx: ctx, parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Markdown{}) }, lookup: func(string) *treenode.BlockTree {
			return &treenode.BlockTree{ID: id, RootID: doc, BoxID: box, Path: tree.Path}
		}, load: func(string, string, *lute.Lute) (*parse.Tree, error) { return tree, nil }, stage: func(*parse.Tree) error { stages++; return nil }, cacheIAL: func(id string, ial map[string]string) { cached[id] = ial }, sync: func() { syncs++ }, indexTree: func(*parse.Tree) { indexes++ }, finish: func(*Transaction) error { return nil }}
		fold := &Operation{Action: "foldHeading", ID: id}
		tx := &Transaction{environment: e, DoOperations: []*Operation{{Action: "setAttrs", ID: id, Data: `{"custom-owner":"` + page + `","bad key":"rejected"}`}, fold}}
		if result := performTx(tx); result != nil {
			t.Fatal(result)
		}
		if stages != 2 || indexes != 1 || syncs != 1 || cached[id]["custom-owner"] != page || cached[id]["fold"] != "1" || cached[id]["bad key"] != "" {
			t.Fatal(stages, indexes, syncs, cached)
		}
		if children, ok := fold.RetData.([]string); !ok || len(children) != 2 {
			t.Fatal(fold.RetData)
		}
		after, err := l.ReadDocument(ctx, box, doc)
		l.Release()
		if err != nil || string(after) != string(raw) {
			t.Fatal("staging test changed source", err)
		}
	}
}
