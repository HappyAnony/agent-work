// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"errors"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func TestReferenceCapabilityRequiresBindingAndJoinsActualWork(t *testing.T) {
	o := syncOptions()
	s, _ := New(o)
	if e := s.RefreshReferences(context.Background(), "id"); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	s.Close(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	o.RefreshReferences = func(ctx context.Context, v conf.AppData, id string) error {
		close(entered)
		<-release
		return ctx.Err()
	}
	s, _ = New(o)
	done := make(chan error, 1)
	go func() { done <- s.RefreshReferences(context.Background(), "id") }()
	<-entered
	short, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Close(short); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	select {
	case <-done:
		t.Fatal("returned before actual handler")
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
