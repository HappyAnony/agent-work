// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func syncOptions() Options {
	return Options{
		Load: func(context.Context) (conf.AppData, error) { return conf.AppData{Sync: conf.NewSync()}, nil },
		Execute: func(context.Context, conf.AppData, Request) (Result, error) {
			return Result{Data: json.RawMessage(`{}`)}, nil
		},
	}
}
func TestFullSyncCriticalCancellationAndActualJoin(t *testing.T) {
	o := syncOptions()
	entered, release := make(chan struct{}), make(chan struct{})
	failure := errors.New("merge failed")
	o.FullSync = func(ctx context.Context, _ conf.AppData, _ FullSyncRequest) error {
		close(entered)
		<-release
		if ctx.Err() != nil {
			t.Error("critical merge was interrupted")
		}
		return failure
	}
	s, _ := New(o)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var result FullSyncResult
	var err error
	go func() { result, err = s.FullSync(ctx, FullSyncRequest{Mode: "normal"}); close(done) }()
	<-entered
	cancel()
	timeout, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if e := s.Close(timeout); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("fake join", e)
	}
	select {
	case <-done:
		t.Fatal("returned while source write still active")
	default:
	}
	close(release)
	<-done
	if !result.Started || !result.CancellationDeferred || !errors.Is(err, failure) || !errors.Is(err, context.Canceled) {
		t.Fatal(result, err)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestFullSyncAdmissionIsolationAndExplicitCapability(t *testing.T) {
	o := syncOptions()
	var calls atomic.Int32
	o.FullSync = func(ctx context.Context, v conf.AppData, r FullSyncRequest) error {
		calls.Add(1)
		v.Sync.CloudName = "changed"
		return nil
	}
	s, _ := New(o)
	defer s.Close(context.Background())
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if out, e := s.FullSync(canceled, FullSyncRequest{Mode: "normal"}); !errors.Is(e, context.Canceled) || out.Started {
		t.Fatal(out, e)
	}
	if _, e := s.FullSync(context.Background(), FullSyncRequest{Mode: "unknown"}); !errors.Is(e, ErrOperation) {
		t.Fatal(e)
	}
	for _, mode := range []string{"normal", "upload", "download", "boot", "exit", "boot-followup"} {
		if _, e := s.FullSync(context.Background(), FullSyncRequest{Mode: mode}); e != nil {
			t.Fatal(mode, e)
		}
	}
	if calls.Load() != 6 {
		t.Fatal(calls.Load())
	}
	detached, _ := New(syncOptions())
	defer detached.Close(context.Background())
	if _, e := detached.FullSync(context.Background(), FullSyncRequest{Mode: "normal"}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
	if e := detached.Repair(context.Background()); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
}
func TestFullSyncSerializesWithRepairAndCanceledWaiter(t *testing.T) {
	o := syncOptions()
	entered, release := make(chan struct{}), make(chan struct{})
	var repairs atomic.Int32
	o.FullSync = func(context.Context, conf.AppData, FullSyncRequest) error { close(entered); <-release; return nil }
	o.Repair = func(context.Context, conf.AppData) error { repairs.Add(1); return nil }
	s, _ := New(o)
	defer s.Close(context.Background())
	done := make(chan error, 1)
	go func() { _, e := s.FullSync(context.Background(), FullSyncRequest{Mode: "normal"}); done <- e }()
	<-entered
	timeout, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Repair(timeout); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if repairs.Load() != 0 {
		t.Fatal("repair raced merge")
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := s.Repair(context.Background()); e != nil || repairs.Load() != 1 {
		t.Fatal(e, repairs.Load())
	}
}
func TestRepairOwnerOnceRetryCancellationAndFailure(t *testing.T) {
	for _, kind := range []string{"error", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			o := syncOptions()
			calls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("repair failed")
			o.Repair = func(context.Context, conf.AppData) error {
				calls++
				if calls == 1 {
					if kind == "cancel" {
						cancel()
						return nil
					}
					return failure
				}
				return nil
			}
			s, _ := New(o)
			defer s.Close(context.Background())
			if e := s.Repair(ctx); e == nil {
				t.Fatal("failed run reported success")
			}
			for i := 0; i < 2; i++ {
				if e := s.Repair(context.Background()); e != nil {
					t.Fatal(e)
				}
			}
			if calls != 2 {
				t.Fatal("not retryable or duplicate run", calls)
			}
			other, _ := New(o)
			defer other.Close(context.Background())
			if e := other.Repair(context.Background()); e != nil || calls != 3 {
				t.Fatal("process-wide once leaked", e, calls)
			}
		})
	}
}
