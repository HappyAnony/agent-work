// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"wiki-kernel/conf"
)

func TestIncrementalIndexCapabilityCancelAndActualJoin(t *testing.T) {
	o := syncOptions()
	unbound, _ := New(o)
	defer unbound.Close(context.Background())
	if e := unbound.IncrementalIndex(context.Background(), IncrementalIndexRequest{}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	entered, release, canceled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	failure := errors.New("flush failed")
	o.IncrementalIndex = func(ctx context.Context, _ conf.AppData, r IncrementalIndexRequest) error {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return failure
	}
	s, _ := New(o)
	done := make(chan error, 1)
	go func() {
		done <- s.IncrementalIndex(context.Background(), IncrementalIndexRequest{Paths: []string{"fixture"}})
	}()
	<-entered
	timeout, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Close(timeout); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("fake join", e)
	}
	<-canceled
	select {
	case <-done:
		t.Fatal("returned with callback active")
	default:
	}
	close(release)
	e := <-done
	if !errors.Is(e, failure) || !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestIncrementalIndexSerializesWithSyncAndRejectsCanceledWaiter(t *testing.T) {
	o := syncOptions()
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	o.FullSync = func(context.Context, conf.AppData, FullSyncRequest) error { close(entered); <-release; return nil }
	o.IncrementalIndex = func(context.Context, conf.AppData, IncrementalIndexRequest) error { calls.Add(1); return nil }
	s, _ := New(o)
	defer s.Close(context.Background())
	done := make(chan error, 1)
	go func() { _, e := s.FullSync(context.Background(), FullSyncRequest{Mode: "normal"}); done <- e }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.IncrementalIndex(ctx, IncrementalIndexRequest{}); !errors.Is(e, context.DeadlineExceeded) || calls.Load() != 0 {
		t.Fatal(e, calls.Load())
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := s.IncrementalIndex(context.Background(), IncrementalIndexRequest{}); e != nil || calls.Load() != 1 {
		t.Fatal(e, calls.Load())
	}
}
