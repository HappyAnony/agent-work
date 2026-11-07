//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/treenode"
)

func TestBookmarkMutationContextActualRenameRemoveAndSQL(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	isolatedConflictQueue(t, flushTx)
	r := env.commitContext.r
	var tree *parse.Tree
	for _, v := range tx.trees {
		tree = v
	}
	n := tree.Root.FirstChild
	n.SetIALAttr("bookmark", "Before")
	io, closeIO, e := r.openRepairIOChecked(context.Background(), true)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.writeRepairTree(context.Background(), io, tree, env.parser()); e != nil {
		t.Fatal(e)
	}
	if e = closeIO(); e != nil {
		t.Fatal(e)
	}
	if e = r.queue.Upsert(context.Background(), tree); e != nil {
		t.Fatal(e)
	}
	if e = r.queue.Flush(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = RenameBookmarkContext(context.Background(), "Before", "After"); e != nil {
		t.Fatal(e)
	}
	loaded, e := r.loadRepairTree(context.Background(), tree.Box, tree.Path, env.parser())
	if e != nil {
		t.Fatal(e)
	}
	if got := treenode.GetNodeInTree(loaded, n.ID).IALAttr("bookmark"); got != "After" {
		t.Fatal(got)
	}
	labels, e := BookmarkLabelsContext(context.Background())
	if e != nil || len(labels) != 1 || labels[0] != "After" {
		t.Fatalf("index stale: %v %v", labels, e)
	}
	if e = RemoveBookmarkContext(context.Background(), "After"); e != nil {
		t.Fatal(e)
	}
	labels, e = BookmarkLabelsContext(context.Background())
	if e != nil || len(labels) != 0 {
		t.Fatalf("remove index stale: %v %v", labels, e)
	}
}

func TestBookmarkMutationContextCancelAndQueryFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := RenameBookmarkContext(ctx, "a", "b"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := RemoveBookmarkContext(ctx, "a"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	_, env, _, _, _ := commitFixture(t)
	isolatedConflictQueue(t, flushTx)
	if _, e := env.commitContext.r.main.DB.Exec("DROP TABLE blocks"); e != nil {
		t.Fatal(e)
	}
	if e := RemoveBookmarkContext(context.Background(), "a"); e == nil {
		t.Fatal("query error hidden")
	}
}
