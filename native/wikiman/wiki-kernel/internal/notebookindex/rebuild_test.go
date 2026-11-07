// SPDX-License-Identifier: AGPL-3.0-or-later
package notebookindex

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"sync/atomic"
	"testing"
	"time"
)

func TestNotebookWorkersCancelAndActuallyJoinBeforeDrain(t *testing.T) {
	entered, release, canceled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var drained atomic.Bool
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	s := Services{Workers: 1, Process: func(c context.Context, _ DocumentPath) ([]*ast.Node, error) {
		close(entered)
		<-c.Done()
		close(canceled)
		<-release
		return nil, c.Err()
	}, Flush: func(c context.Context) error {
		if c.Err() != nil {
			t.Error("drain canceled")
		}
		drained.Store(true)
		return nil
	}, Bind: func(context.Context, []*ast.Node) error { t.Error("bound after cancellation"); return nil }, History: func(context.Context) error { t.Error("history marked after cancellation"); return nil }}
	go func() { done <- Rebuild(ctx, []DocumentPath{{ID: "first"}, {ID: "second"}}, s) }()
	<-entered
	cancel()
	<-canceled
	select {
	case <-done:
		t.Fatal("fake join")
	case <-time.After(10 * time.Millisecond):
	}
	if drained.Load() {
		t.Fatal("drained while worker active")
	}
	close(release)
	if e := <-done; !errors.Is(e, context.Canceled) || !drained.Load() {
		t.Fatal(e)
	}
}
func TestNotebookFailurePanicAndHooksOnlyAfterFlush(t *testing.T) {
	for _, mode := range []string{"success", "load-error", "worker-panic", "flush-error", "bind-error", "bind-panic"} {
		t.Run(mode, func(t *testing.T) {
			var order []string
			failure := errors.New("fixture failed")
			s := Services{Workers: 1, Process: func(context.Context, DocumentPath) ([]*ast.Node, error) {
				order = append(order, "process")
				if mode == "load-error" {
					return nil, failure
				}
				if mode == "worker-panic" {
					panic("fixture")
				}
				return []*ast.Node{{Type: ast.NodeAttributeView}}, nil
			}, Flush: func(context.Context) error {
				order = append(order, "flush")
				if mode == "flush-error" {
					return failure
				}
				return nil
			}, Bind: func(_ context.Context, n []*ast.Node) error {
				order = append(order, "bind")
				if len(n) != 1 {
					t.Error("missing AV nodes")
				}
				if mode == "bind-error" {
					return failure
				}
				if mode == "bind-panic" {
					panic("fixture")
				}
				return nil
			}, History: func(context.Context) error { order = append(order, "history"); return nil }}
			e := Rebuild(context.Background(), []DocumentPath{{ID: "fixture"}}, s)
			if mode == "success" {
				if e != nil || len(order) != 4 || order[3] != "history" {
					t.Fatal(order, e)
				}
			} else {
				if e == nil || order[len(order)-1] == "history" {
					t.Fatal(order, e)
				}
			}
			if len(order) < 2 || order[1] != "flush" {
				t.Fatal("missing error drain", order)
			}
		})
	}
	if e := Rebuild(context.Background(), nil, Services{}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
}
