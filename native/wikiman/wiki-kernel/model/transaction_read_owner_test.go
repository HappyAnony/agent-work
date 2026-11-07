//go:build fts5

package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"os"
	"path/filepath"
	"testing"
)

func TestTransactionReadAndNoticeOwnerRejectsStaleAndCanceled(t *testing.T) {
	_, env, _, _, _ := commitFixture(t)
	published := 0
	env.updateUnfold = func(context.Context, *ast.Node, *ast.Node) error { published++; return nil }
	env.updateEvent = func(context.Context, []*Transaction) error { published++; return nil }
	bindTransactionReadAndNoticePorts(env, env.commitContext)
	calls := []func(context.Context) error{
		func(ctx context.Context) error { _, e := env.referenceCounts(ctx, []string{"id"}); return e },
		func(ctx context.Context) error { _, e := env.avNames(ctx, ""); return e },
		func(ctx context.Context) error { return env.updateUnfold(ctx, nil, nil) },
		func(ctx context.Context) error { return env.updateEvent(ctx, nil) },
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i, call := range calls {
		if e := call(ctx); !errors.Is(e, context.Canceled) {
			t.Fatalf("port %d lost cancellation: %v", i, e)
		}
		old := Conf
		Conf = &AppConf{}
		e := call(context.Background())
		Conf = old
		if e == nil {
			t.Fatalf("port %d accepted stale owner", i)
		}
	}
	if published != 0 {
		t.Fatal("stale notification published")
	}
	if e := env.updateUnfold(context.Background(), nil, nil); e != nil {
		t.Fatal(e)
	}
	if e := env.updateEvent(context.Background(), nil); e != nil {
		t.Fatal(e)
	}
	if published != 2 {
		t.Fatal("current notifications not published")
	}
}

func TestTransactionBoundReadsReturnActualErrors(t *testing.T) {
	_, env, _, _, _ := commitFixture(t)
	ctx := context.Background()
	avDir := filepath.Join(env.commitContext.r.data, "storage", "av")
	if e := os.MkdirAll(avDir, 0700); e != nil {
		t.Fatal(e)
	}
	const id = "20261104090000-names01"
	if e := os.WriteFile(filepath.Join(avDir, id+".json"), []byte(`{"name":"Owner name"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if names, e := env.avNames(ctx, id); e != nil || names != `<span data-av-id="20261104090000-names01" data-popover-url="/api/av/getMirrorDatabaseBlocks" class="popover__block">Owner name</span>` {
		t.Fatalf("name read: %q %v", names, e)
	}
	if counts, e := env.referenceCounts(ctx, nil); e != nil || len(counts) != 0 {
		t.Fatalf("empty counts: %v %v", counts, e)
	}
	if names, e := env.avNames(ctx, ""); e != nil || names != "" {
		t.Fatalf("empty names: %q %v", names, e)
	}
	if names, e := env.avNames(ctx, "20261104090000-missing"); e == nil || names != "" {
		t.Fatalf("missing AV was hidden: %q %v", names, e)
	}
	if _, e := env.commitContext.r.main.DB.Exec("DROP TABLE refs"); e != nil {
		t.Fatal(e)
	}
	if counts, e := env.referenceCounts(ctx, []string{"id"}); e == nil || counts != nil {
		t.Fatalf("SQL error hidden: %v %v", counts, e)
	}
}
