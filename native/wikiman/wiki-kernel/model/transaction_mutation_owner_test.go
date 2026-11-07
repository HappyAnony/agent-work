//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestTransactionMutationPortsRejectCanceledAndReplacedOwner(t *testing.T) {
	_, env, _, _, _ := commitFixture(t)
	reads := 0
	env.updateDynamicText = func(context.Context, string) (string, error) { reads++; return "captured", nil }
	bindTransactionMutationPorts(env, env.commitContext)
	ports := []func(context.Context) error{
		func(ctx context.Context) error { return env.deleteBlockTree(ctx, "id") },
		func(ctx context.Context) error { return env.insertAVRegistry(ctx, &ast.Node{Type: ast.NodeParagraph}) },
		func(ctx context.Context) error { _, e := env.updateDynamicText(ctx, "id"); return e },
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i, port := range ports {
		if e := port(ctx); !errors.Is(e, context.Canceled) {
			t.Fatalf("port %d cancel: %v", i, e)
		}
		old := Conf
		Conf = &AppConf{}
		e := port(context.Background())
		Conf = old
		if e == nil {
			t.Fatalf("port %d accepted replacement", i)
		}
	}
	if reads != 0 {
		t.Fatal("dynamic cache read after rejection")
	}
	if text, e := env.updateDynamicText(context.Background(), "id"); e != nil || text != "captured" {
		t.Fatalf("dynamic read %q %v", text, e)
	}
}

func TestTransactionBoundBlockTreeDeletionAndFailure(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	r := env.commitContext.r
	ctx := context.Background()
	var id string
	for _, tree := range tx.trees {
		if e := r.registry.Upsert(ctx, tree); e != nil {
			t.Fatal(e)
		}
		id = tree.Root.ID
	}
	if id == "" {
		t.Fatal("empty fixture")
	}
	if e := env.deleteBlockTree(ctx, id); e != nil {
		t.Fatal(e)
	}
	rows, e := r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
	if e != nil || len(rows) != 0 {
		t.Fatalf("delete failed: %v %v", rows, e)
	}
	if _, e = r.registry.DB.Exec("DROP TABLE blocktrees"); e != nil {
		t.Fatal(e)
	}
	if e = env.deleteBlockTree(ctx, id); e == nil {
		t.Fatal("SQL failure hidden")
	}
}

func TestTransactionBoundAVRegistryRejectsInvalidNode(t *testing.T) {
	_, env, _, _, _ := commitFixture(t)
	if e := env.insertAVRegistry(context.Background(), nil); e == nil {
		t.Fatal("invalid node failure hidden")
	}
	if e := env.insertAVRegistry(context.Background(), &ast.Node{Type: ast.NodeParagraph}); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(env.commitContext.r.data, "storage", "av", "blocks.msgpack")
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		t.Fatal(e)
	}
	node := &ast.Node{Type: ast.NodeAttributeView, ID: ast.NewNodeID(), AttributeViewID: ast.NewNodeID()}
	if e := env.insertAVRegistry(context.Background(), node); e != nil {
		t.Fatal(e)
	}
	rows := mirrorRows(t, file)
	if len(rows[node.AttributeViewID]) != 1 || rows[node.AttributeViewID][0] != node.ID {
		t.Fatal("captured mirror registration missing", rows)
	}
	if e := os.WriteFile(file, []byte("corrupt registry"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := env.insertAVRegistry(context.Background(), node); e == nil {
		t.Fatal("registry read failure hidden")
	}
}
