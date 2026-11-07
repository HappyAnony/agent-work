package model

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/treenode"
)

func TestOriginalInsertRejectsUnboundBeforeBegin(t *testing.T) {
	for _, missing := range []string{"assets", "refs", "registry", "bind"} {
		calls := 0
		e := &transactionEnvironment{ctx: context.Background(), parser: func() *lute.Lute { calls++; return nativeparser.NewLute(nativeparser.Defaults()) }, lookup: func(string) *treenode.BlockTree { calls++; return nil }, load: func(string, string, *lute.Lute) (*parse.Tree, error) { calls++; return nil, nil }, stage: func(*parse.Tree) error { calls++; return nil }, cacheIAL: func(string, map[string]string) { calls++ }, finish: func(*Transaction) error { calls++; return nil }, insertAssets: func(context.Context, *parse.Tree, *Transaction) error { return nil }, insertReferences: func(context.Context, ...string) error { return nil }, insertAVRegistry: func(context.Context, *ast.Node) error { return nil }, insertAVBind: func(context.Context, *ast.Node, *parse.Tree, *Transaction) error { return nil }}
		switch missing {
		case "assets":
			e.insertAssets = nil
		case "refs":
			e.insertReferences = nil
		case "registry":
			e.insertAVRegistry = nil
		case "bind":
			e.insertAVBind = nil
		}
		tx := &Transaction{environment: e, DoOperations: []*Operation{{Action: "insert"}}}
		if got := performTx(tx); got == nil || calls != 0 || tx.state.Load() != 0 {
			t.Fatal(missing, got, calls, tx.state.Load())
		}
	}
}
