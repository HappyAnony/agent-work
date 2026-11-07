package cloudrepo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func TestRestoreExplicitAuthorityAndPartialResultJoin(t *testing.T) {
	o := checkoutFixtureOptions()
	s, _ := New(o)
	if _, e := s.Restore(context.Background(), strings.Repeat("a", 40)); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	s.Close(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	o.Restore = func(ctx context.Context, _ conf.AppData, _ string) (RestoreResult, error) {
		close(entered)
		<-release
		return RestoreResult{Applied: true, Reindexed: true, SyncPaused: true, CancellationDeferred: true}, ctx.Err()
	}
	s, _ = New(o)
	type result struct {
		v RestoreResult
		e error
	}
	done := make(chan result, 1)
	go func() { v, e := s.Restore(context.Background(), strings.Repeat("a", 40)); done <- result{v, e} }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Close(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	close(release)
	v := <-done
	if !v.v.Applied || !v.v.Reindexed || !v.v.SyncPaused || !v.v.CancellationDeferred || !errors.Is(v.e, context.Canceled) {
		t.Fatal(v)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
