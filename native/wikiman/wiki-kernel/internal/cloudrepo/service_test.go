package cloudrepo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func TestQuerySnapshotAdmissionCloseAndNoFakeJoin(t *testing.T) {
	cfg := conf.AppData{Sync: conf.NewSync()}
	entered, release := make(chan struct{}), make(chan struct{})
	calls := 0
	s, e := New(Options{Load: func(context.Context) (conf.AppData, error) { return cfg, nil }, Execute: func(ctx context.Context, v conf.AppData, r Request) (Result, error) {
		calls++
		v.Sync.CloudName = "private"
		close(entered)
		<-release
		return Result{Data: json.RawMessage(`{}`)}, ctx.Err()
	}})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := s.Query(context.Background(), Request{Kind: "space"}); done <- e }()
	<-entered
	c, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, e = s.Query(c, Request{Kind: "space"}); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	c2, cancel2 := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel2()
	if e = s.Close(c2); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	close(release)
	if e = <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if cfg.Sync.CloudName != "main" || calls != 1 {
		t.Fatal("aliased snapshot or canceled waiter executed", cfg.Sync.CloudName, calls)
	}
	if _, e = s.Query(context.Background(), Request{Kind: "space"}); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
}
func TestRejectMutationUnknownAndBadCapabilities(t *testing.T) {
	if _, e := New(Options{}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	s, _ := New(Options{Load: func(context.Context) (conf.AppData, error) {
		t.Fatal("invalid operation loaded configuration")
		return conf.AppData{}, nil
	}, Execute: func(context.Context, conf.AppData, Request) (Result, error) {
		t.Fatal("invalid operation performed IO")
		return Result{}, nil
	}})
	defer s.Close(context.Background())
	for _, kind := range []string{"purge", "checkout", "upload", "remove", "create"} {
		if _, e := s.Query(context.Background(), Request{Kind: kind}); !errors.Is(e, ErrOperation) {
			t.Fatal(kind, e)
		}
	}
	if _, e := s.Query(context.Background(), Request{Kind: "logs", Page: 1000001}); !errors.Is(e, ErrOperation) {
		t.Fatal(e)
	}
}
