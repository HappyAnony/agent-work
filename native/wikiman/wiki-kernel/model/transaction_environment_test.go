package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/dataparser"
	"os"
	"testing"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/internal/workspace"
	"wiki-kernel/treenode"
)

// Nonempty original performTx call-chain, backed by a real Context read lease.
// Finish is deliberately staging-only: this is not a source-save receipt test.
func TestOriginalPerformTxUsesOwnedLoadStageAndFinish(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	m := workspace.NewManager(workspace.Options{})
	store, err := m.OpenStore(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Drain(ctx)
	l, err := m.Acquire(ctx, store, "1760000000000000002")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	const box = "20261019090000-box0001"
	const doc = "20261019090001-doc0001"
	const block = "20261019090002-par0001"
	raw, err := l.ReadDocument(ctx, box, doc)
	if err != nil {
		t.Fatal(err)
	}
	var loads, stages, finishes int
	build := func(finishErr error) *Transaction {
		e := &transactionEnvironment{ctx: ctx, parser: func() *lute.Lute { return nativeparser.NewLute(nativeparser.Markdown{}) },
			lookup: func(id string) *treenode.BlockTree {
				if id != block {
					return nil
				}
				return &treenode.BlockTree{ID: block, RootID: doc, BoxID: box, Path: "/" + doc + ".docj"}
			},
			load: func(b, p string, engine *lute.Lute) (*parse.Tree, error) {
				loads++
				data, err := l.ReadDocument(ctx, b, doc)
				if err != nil {
					return nil, err
				}
				tree, _, err := dataparser.ParseJSON(data, engine.ParseOptions)
				if tree != nil {
					tree.ID = doc
					tree.Box = b
					tree.Path = p
				}
				return tree, err
			},
			stage: func(tree *parse.Tree) error {
				stages++
				if treenode.GetNodeInTree(tree, block).IALAttr("updated") != "20261021123000" {
					t.Error("original operation was not applied")
				}
				return nil
			},
			cacheIAL: func(string, map[string]string) {},
			finish: func(tx *Transaction) error {
				finishes++
				if len(tx.trees) != 1 {
					t.Error("missing staged tree")
				}
				return finishErr
			},
		}
		return &Transaction{environment: e, DoOperations: []*Operation{{Action: "doUpdateUpdated", ID: block, Data: "20261021123000"}}}
	}
	tx := build(nil)
	if result := performTx(tx); result != nil {
		t.Fatal(result)
	}
	if loads != 1 || stages != 1 || finishes != 1 || tx.state.Load() != 2 {
		t.Fatal(loads, stages, finishes, tx.state.Load())
	}
	expected := errors.New("synthetic commit failure")
	tx = build(expected)
	if result := performTx(tx); result == nil || result.msg != expected.Error() || tx.state.Load() != 3 {
		t.Fatal(result, tx.state.Load())
	}
	if !tx.m.TryLock() {
		t.Fatal("failed commit leaked transaction lock")
	}
	tx.m.Unlock()
	tx = build(nil)
	tx.DoOperations[0].Action = "move"
	if result := performTx(tx); result == nil || tx.state.Load() != 0 {
		t.Fatal("unmigrated operation entered scoped environment", result)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	tx = build(nil)
	tx.environment.ctx = cancelled
	if result := performTx(tx); result == nil || tx.state.Load() != 0 {
		t.Fatal("cancelled transaction began", result)
	}
	after, err := l.ReadDocument(ctx, box, doc)
	if err != nil || string(after) != string(raw) {
		t.Fatal("staging test wrote source", err)
	}
}
