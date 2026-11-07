//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/treenode"
)

func TestTransactionLookupActualRegistryAndSQLFailure(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	var tree *parse.Tree
	for _, v := range tx.trees {
		tree = v
	}
	if e := env.stage(tree); e != nil {
		t.Fatal(e)
	}
	block, e := env.lookupBlock(tree.ID)
	if e != nil || block == nil || block.RootID != tree.ID {
		t.Fatalf("lookup failed: %+v %v", block, e)
	}
	if block, e = env.lookupBlock("missing"); e != nil || block != nil {
		t.Fatalf("missing result: %+v %v", block, e)
	}
	if _, e = env.commitContext.r.registry.DB.Exec("DROP TABLE blocktrees"); e != nil {
		t.Fatal(e)
	}
	if block, e = env.lookupBlock(tree.ID); e == nil || block != nil {
		t.Fatalf("SQL failure hidden: %+v %v", block, e)
	}
}

func TestTransactionInsertLookupsPropagateFailureWithoutReload(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	want := errors.New("lookup failed")
	env.lookupChecked = func(context.Context, string) (*treenode.BlockTree, error) { return nil, want }
	reloads := 0
	env.reloadUI = func() { reloads++ }
	for _, action := range []string{"prepend", "append", "insert"} {
		op := &Operation{ParentID: "id"}
		var e *TxErr
		switch action {
		case "prepend":
			e = tx.doPrependInsert(op)
		case "append":
			e = tx.doAppendInsert(op)
		case "insert":
			e = tx.doInsert(op)
		}
		if e == nil || e.msg != want.Error() {
			t.Fatalf("%s failure lost: %+v", action, e)
		}
	}
	if reloads != 0 {
		t.Fatal("SQL failure treated as missing target reload")
	}
}

func TestTransactionCachedTreeAndLateLoadRecheckOwner(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	var tree *parse.Tree
	for _, v := range tx.trees {
		tree = v
	}
	bt := &treenode.BlockTree{ID: tree.ID, RootID: tree.ID, BoxID: tree.Box, Path: tree.Path}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env.ctx = ctx
	if _, e := tx.loadTreeByBlockTree(bt); !errors.Is(e, context.Canceled) {
		t.Fatalf("cached cancellation ignored: %v", e)
	}
	env.ctx = context.Background()
	old := Conf
	Conf = &AppConf{}
	_, e := tx.loadTreeByBlockTree(bt)
	Conf = old
	if e == nil {
		t.Fatal("cached tree accepted replacement owner")
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	env.ctx = ctx
	tx.trees = map[string]*parse.Tree{}
	env.load = func(string, string, *lute.Lute) (*parse.Tree, error) { cancel(); return tree, nil }
	if _, e = tx.loadTreeByBlockTree(bt); !errors.Is(e, context.Canceled) {
		t.Fatalf("late load cancellation ignored: %v", e)
	}
	if len(tx.trees) != 0 {
		t.Fatal("cancelled load entered transaction cache")
	}
}
