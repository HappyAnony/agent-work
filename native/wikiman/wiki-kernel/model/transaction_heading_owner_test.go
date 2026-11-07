//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
)

func TestTransactionHeadingEffectsOwnerAndFailure(t *testing.T) {
	c, env, tx, _, _ := commitFixture(t)
	var tree *parse.Tree
	for _, v := range tx.trees {
		tree = v
	}
	c.syncRuntime.same.Store(42)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := env.headingEffects(ctx, tree); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancel lost: %v", e)
	}
	old := Conf
	Conf = &AppConf{}
	e := env.headingEffects(context.Background(), tree)
	Conf = old
	if e == nil {
		t.Fatal("stale owner admitted index")
	}
	if e = env.headingEffects(context.Background(), nil); e == nil {
		t.Fatal("invalid queue operation hidden")
	}
	if c.syncRuntime.same.Load() != 42 {
		t.Fatal("sync planned after rejection")
	}
	if e = env.headingEffects(context.Background(), tree); e != nil {
		t.Fatal(e)
	}
	if c.syncRuntime.same.Load() != 0 {
		t.Fatal("captured owner sync not planned")
	}
	if e = env.commitContext.r.queue.Flush(context.Background()); e != nil {
		t.Fatal(e)
	}
}

func TestTransactionFoldAndUnfoldPropagateEffectFailure(t *testing.T) {
	for _, action := range []string{"fold", "unfold"} {
		t.Run(action, func(t *testing.T) {
			_, env, tx, _, _ := commitFixture(t)
			env.editorParser = env.parser
			var tree *parse.Tree
			for _, v := range tx.trees {
				tree = v
			}
			n := tree.Root.FirstChild
			n.Type = ast.NodeHeading
			n.HeadingLevel = 1
			want := errors.New("heading effect failed")
			env.headingEffects = func(context.Context, *parse.Tree) error { return want }
			op := &Operation{ID: n.ID}
			var got *TxErr
			if action == "fold" {
				got = tx.doFoldHeading(op)
			} else {
				got = tx.doUnfoldHeading(op)
			}
			if got == nil || got.msg != want.Error() || op.RetData != nil {
				t.Fatalf("effect failure hidden: %+v", got)
			}
		})
	}
}
