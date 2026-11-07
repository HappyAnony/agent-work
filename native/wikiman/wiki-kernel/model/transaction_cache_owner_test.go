//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/cache"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
)

func TestTransactionCachePortsStageAndFailures(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	var tree *parse.Tree
	for _, v := range tx.trees {
		tree = v
	}
	if e := env.stage(tree); e != nil {
		t.Fatal(e)
	}
	rows, e := env.commitContext.r.registry.Query(context.Background(), indexdb.BlockTreeFilter{Kind: "id", Value: tree.ID})
	if e != nil || len(rows) != 1 {
		t.Fatalf("stage missing: %v %v", rows, e)
	}
	refNode := &ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: "20261104090000-ref0001", TextMarkTextContent: "definition"}
	tree.Root.FirstChild.AppendChild(refNode)
	if e = env.updateCacheRef(context.Background(), tree, refNode); e != nil {
		t.Fatal(e)
	}
	refs, e := sql.CaptureCommitReferences()(context.Background(), refNode.TextMarkBlockRefID)
	if e != nil || len(refs) != 1 || refs[0].Content != "definition" || refs[0].RootID != tree.ID {
		t.Fatalf("reference projection missing: %v %v", refs, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = env.updateCacheRef(ctx, nil, nil); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancel lost: %v", e)
	}
	if e = env.updateCacheRef(context.Background(), nil, nil); e == nil {
		t.Fatal("projection failure hidden")
	}
	if _, e = env.commitContext.r.registry.DB.Exec("DROP TABLE blocktrees"); e != nil {
		t.Fatal(e)
	}
	if e = env.stage(tree); e == nil {
		t.Fatal("stage SQL failure hidden")
	}
}

func TestTransactionCachePortsPinIALAndReferencePublisher(t *testing.T) {
	_, env, _, _, _ := commitFixture(t)
	cache.CloseLegacyIAL()
	if e := cache.OpenLegacyIAL(); e != nil {
		t.Fatal(e)
	}
	env.cacheIAL("detached-id", map[string]string{"name": "old"})
	if got := cache.GetBlockIAL("detached-id"); got["name"] == "old" {
		t.Fatal("old publisher followed replacement")
	}
	publish := sql.CaptureReferencePublisher()
	sql.CloseLegacyCaches()
	if e := sql.OpenLegacyCaches(); e != nil {
		t.Fatal(e)
	}
	if e := publish(context.Background(), indexdb.RefRecord{ID: ast.NewNodeID()}); e == nil {
		t.Fatal("old reference publisher followed replacement")
	}
}
