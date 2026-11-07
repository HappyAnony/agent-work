package model

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestFSNotifyOwnerCancelsJoinsAndRetainsRunError(t *testing.T) {
	entered := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	runErr := errors.New("watch callback failed")
	owner, err := openFSNotifyOwner(t.TempDir(), func(ctx context.Context, _ *fsnotify.Watcher) error {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return runErr
	})
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	deadline, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err = owner.Close(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("active callback reported drained: %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel callback context")
	}
	close(release)
	if err = owner.Close(context.Background()); !errors.Is(err, runErr) {
		t.Fatalf("run error lost after retry: %v", err)
	}
}

func TestFSNotifyOwnerReportsPanicAndInitializationFailure(t *testing.T) {
	if owner, err := openFSNotifyOwner(t.TempDir()+"/missing", func(context.Context, *fsnotify.Watcher) error { return nil }); err == nil || owner != nil {
		t.Fatal("missing watcher root was accepted")
	}
	owner, err := openFSNotifyOwner(t.TempDir(), func(context.Context, *fsnotify.Watcher) error {
		panic("synthetic")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(context.Background()); err == nil || !strings.Contains(err.Error(), "synthetic") {
		t.Fatalf("panic was not retained: %v", err)
	}
}
