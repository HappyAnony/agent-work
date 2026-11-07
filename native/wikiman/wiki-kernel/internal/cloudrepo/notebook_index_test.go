// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"errors"
	"testing"
	"time"

	"wiki-kernel/conf"
)

func TestNotebookCapabilitySerialAdmissionCancellationAndJoin(t *testing.T) {
	o := syncOptions()
	s, _ := New(o)
	if e := s.NotebookIndex(context.Background(), NotebookIndexRequest{Kind: "index"}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	s.Close(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	o.NotebookIndex = func(ctx context.Context, _ conf.AppData, _ NotebookIndexRequest) error {
		close(entered)
		<-ctx.Done()
		<-release
		return ctx.Err()
	}
	s, _ = New(o)
	if e := s.NotebookIndex(context.Background(), NotebookIndexRequest{Kind: "bad"}); !errors.Is(e, ErrOperation) {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- s.NotebookIndex(context.Background(), NotebookIndexRequest{Kind: "index"}) }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, e := s.FullSync(ctx, FullSyncRequest{Mode: "normal"}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	if e := s.NotebookIndex(ctx, NotebookIndexRequest{Kind: "refs"}); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	closeCtx, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if e := s.Close(closeCtx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("fake join", e)
	}
	select {
	case <-done:
		t.Fatal("returned with active work")
	default:
	}
	close(release)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
