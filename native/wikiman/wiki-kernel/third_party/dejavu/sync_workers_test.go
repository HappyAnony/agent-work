// SPDX-License-Identifier: AGPL-3.0-or-later
package dejavu

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestSyncWorkersFailureCancelPanicJoinActualRunning(t *testing.T) {
	for _, mode := range []string{"error", "panic", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, release, failed := make(chan struct{}), make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			failure := errors.New("provider failed")
			go func() {
				done <- joinedSyncBatch(ctx, 2, []string{"a", "b"}, func(id string, _ int) error {
					if id == "a" {
						<-entered
						close(failed)
						switch mode {
						case "error":
							return failure
						case "panic":
							panic("provider panic")
						case "cancel":
							cancel()
							return ctx.Err()
						}
					}
					close(entered)
					<-release
					return nil
				})
			}()
			<-failed
			select {
			case e := <-done:
				t.Fatal("returned before actual running provider completed", e)
			default:
			}
			close(release)
			if e := <-done; e == nil {
				t.Fatal("false success")
			}
		})
	}
}
func TestSyncWorkersBoundedDispatchAndNoWorkOnCanceledParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := joinedSyncBatch(ctx, 4, []string{"x"}, func(string, int) error { t.Error("canceled worker started"); return nil }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	var mu sync.Mutex
	got := map[string]bool{}
	if e := joinedSyncBatch(context.Background(), 0, []string{"a", "b", "c"}, func(id string, _ int) error { mu.Lock(); got[id] = true; mu.Unlock(); return nil }); e != nil || len(got) != 3 {
		t.Fatal(e, got)
	}
}
