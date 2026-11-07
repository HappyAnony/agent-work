package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

func TestOriginalAppendInsertPrependAndAppendContextActualSource(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if e := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); e != nil {
		t.Fatal(e)
	}
	const pageA = "1760000000000000002"
	const pageB = "1760000000000000003"
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"
	const id = "20261019090002-par0001"
	const inserted = "20261022090001-ins0001"
	bData := filepath.Join(root, "pages", pageB, "data")
	if e := os.MkdirAll(bData, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.CopyFS(bData, os.DirFS(filepath.Join(root, "pages", pageA, "data"))); e != nil {
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
		defer lease.Release()
		before, e := lease.ReadDocument(ctx, box, doc)
		if e != nil {
			t.Fatal(e)
		}
		for _, action := range []string{"prependInsert", "appendInsert", "append"} {
			for _, failure := range []string{"", "load", "stage", "finish", "missing"} {
				t.Run(page+"/"+action+"/"+failure, func(t *testing.T) {
					var candidate *parse.Tree
					stages, finishes := 0, 0
					ials := map[string]map[string]string{}
					want := errors.New("synthetic " + failure)
					env := &transactionEnvironment{ctx: ctx, parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) }, lookup: func(key string) *treenode.BlockTree {
						if failure == "missing" {
							return nil
						}
						return &treenode.BlockTree{ID: key, RootID: doc, BoxID: box, Path: "/" + doc + ".docj"}
					}, load: func(b, p string, engine *lute.Lute) (*parse.Tree, error) {
						if failure == "load" {
							return nil, want
						}
						raw, e := lease.ReadDocument(ctx, b, doc)
						if e != nil {
							return nil, e
						}
						tree, e := dataparser.ParseJSONWithoutFix(raw, engine.ParseOptions)
						if e != nil {
							return nil, e
						}
						tree.ID, tree.Box, tree.Path = doc, b, p
						return tree, nil
					}, stage: func(tree *parse.Tree) error {
						stages++
						candidate = tree
						if failure == "stage" {
							return want
						}
						return nil
					}, cacheIAL: func(id string, attrs map[string]string) { ials[id] = attrs }, finish: func(tx *Transaction) error {
						finishes++
						if failure == "finish" {
							return want
						}
						return nil
					}}
					op := &Operation{Action: action, ParentID: id, Data: `<div data-type="NodeParagraph" data-node-id="` + inserted + `"><div contenteditable="true">` + page + ` inserted</div></div>`}
					if action == "append" {
						op.ID = id
						op.ParentID = doc
						op.Data = nil
					}
					tx := &Transaction{environment: env, DoOperations: []*Operation{op}}
					result := performTx(tx)
					if failure == "" {
						if result != nil || stages != 1 || finishes != 1 || tx.state.Load() != 2 {
							t.Fatal(result, stages, finishes, tx.state.Load())
						}
						if action == "append" {
							if candidate.Root.LastChild.ID != id {
								t.Fatal("append lost original move semantics")
							}
						} else {
							n := treenode.GetNodeInTree(candidate, inserted)
							if n == nil || n.Parent.ID != doc || op.Action != "insert" || op.ID != inserted || op.ParentID != doc || !strings.Contains(n.Text(), page) || len(ials) == 0 {
								t.Fatal(n, op, ials)
							}
						}
					} else if result == nil || tx.state.Load() != 3 {
						t.Fatal("failed operation not rolled back", result, tx.state.Load())
					}
					if !tx.m.TryLock() {
						t.Fatal("transaction lock retained")
					}
					tx.m.Unlock()
					after, e := lease.ReadDocument(ctx, box, doc)
					if e != nil || string(after) != string(before) {
						t.Fatal("candidate/staging wrote source", e)
					}
				})
			}
		}
	}
}

func TestOriginalAppendContextRejectsOtherReceivers(t *testing.T) {
	// Unreviewed receivers, and move without its explicit services, stay closed.
	env := &transactionEnvironment{ctx: context.Background(), parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Defaults()) }, lookup: func(string) *treenode.BlockTree { return nil }, load: func(string, string, *lute.Lute) (*parse.Tree, error) { return nil, nil }, stage: func(*parse.Tree) error { return nil }, cacheIAL: func(string, map[string]string) {}, finish: func(*Transaction) error { return nil }}
	for _, action := range []string{"update", "insert", "delete", "move", "updateAttrViewCell"} {
		if env.validate([]*Operation{{Action: action}}) == nil {
			t.Fatal("unaudited receiver allowed", action)
		}
	}
}
