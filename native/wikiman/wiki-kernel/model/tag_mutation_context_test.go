//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/av"
)

func TestTagMutationContextActualRenameAndExactRemove(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	isolatedConflictQueue(t, flushTx)
	var tree *parse.Tree
	for _, v := range tx.trees {
		tree = v
	}
	tree.Root.SetIALAttr("tags", "Before,Before/sub,Keep")
	base := bookmarkMutationWithOwner(context.Background(), context.Background(), env.commitContext)
	if e := base.stage(tree); e != nil {
		t.Fatal(e)
	}
	if e := RenameTagContext(context.Background(), "Before", "After"); e != nil {
		t.Fatal(e)
	}
	read := func() string {
		loaded, e := env.commitContext.r.loadRepairTree(context.Background(), tree.Box, tree.Path, env.parser())
		if e != nil {
			t.Fatal(e)
		}
		return loaded.Root.IALAttr("tags")
	}
	if got := read(); got != "After,After/sub,Keep" {
		t.Fatal(got)
	}
	if e := RemoveTagContext(context.Background(), "After"); e != nil {
		t.Fatal(e)
	}
	if got := read(); got != "After/sub,Keep" {
		t.Fatal(got)
	}
}

func TestTagMutationContextCancelQueryAndAVOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := RenameTagContext(ctx, "a", "b"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := RemoveTagContext(ctx, "a"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	_, env, _, _, _ := commitFixture(t)
	e := tagMutationWithOwner(context.Background(), context.Background(), env.commitContext)
	if _, err := env.commitContext.r.main.DB.Exec("DROP TABLE spans"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.query("a"); err == nil {
		t.Fatal("query failure hidden")
	}
}

func TestTagAVTextEnvironmentUsesCapturedTransaction(t *testing.T) {
	tx, a, _, _ := duplicateContextFixture(t, av.LayoutTypeTable)
	old := Conf
	Conf = &AppConf{}
	defer func() { Conf = old }()
	if _, err := tx.attributeViewTextEnvironment().read(a.ID); err == nil {
		t.Fatal("AV text followed replacement owner")
	}
}
