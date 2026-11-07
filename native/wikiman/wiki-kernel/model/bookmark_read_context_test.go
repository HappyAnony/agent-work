//go:build fts5

package model

import (
	"context"
	"errors"
	"testing"
)

func TestBookmarkReadContextActualQueriesAndErrors(t *testing.T) {
	_, env, _, _, _ := commitFixture(t)
	isolatedConflictQueue(t, flushTx)
	if _, e := env.commitContext.r.main.DB.Exec(`UPDATE blocks SET ial='{: bookmark="Owned"}',name='Owner name'`); e != nil {
		t.Fatal(e)
	}
	labels, e := BookmarkLabelsContext(context.Background())
	if e != nil || len(labels) != 1 || labels[0] != "Owned" {
		t.Fatalf("labels %v %v", labels, e)
	}
	panel, e := BuildBookmarkContext(context.Background())
	if e != nil || panel == nil || len(*panel) != 1 || (*panel)[0].Name != "Owned" {
		t.Fatalf("panel %v %v", panel, e)
	}
	if _, e = env.commitContext.r.main.DB.Exec("DROP TABLE blocks"); e != nil {
		t.Fatal(e)
	}
	if _, e = BookmarkLabelsContext(context.Background()); e == nil {
		t.Fatal("label SQL failure hidden")
	}
	if _, e = BuildBookmarkContext(context.Background()); e == nil {
		t.Fatal("panel SQL failure hidden")
	}
}

func TestBookmarkReadContextCancellationAndTransactionOwner(t *testing.T) {
	_, env, tx, _, _ := commitFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := BookmarkLabelsContext(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := BuildBookmarkContext(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	// A worker-bound environment must not drain the FIFO containing itself.
	reader := bookmarkReadWithOwner(context.Background(), env.commitContext, tx)
	if e := reader.wait(); e != nil {
		t.Fatal(e)
	}
	old := Conf
	Conf = &AppConf{}
	_, e := tx.bookmarkLabels()
	Conf = old
	if e == nil {
		t.Fatal("transaction labels followed replacement owner")
	}
}
