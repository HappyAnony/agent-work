package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"wiki-kernel/internal/taskgroup"
)

func TestLegacyBootOwnerCancelsJoinsAndRejectsLateWork(t *testing.T) {
	legacyRuntimeLock.Lock()
	oldTasks, oldStarted, oldClosed := legacyBootTasks, legacyRuntimeStarted, legacyRuntimeClosed
	legacyBootTasks = taskgroup.Open()
	legacyRuntimeStarted, legacyRuntimeClosed = true, false
	legacyRuntimeLock.Unlock()
	t.Cleanup(func() {
		_ = closeLegacyBootTasks(context.Background())
		legacyRuntimeLock.Lock()
		legacyBootTasks, legacyRuntimeStarted, legacyRuntimeClosed = oldTasks, oldStarted, oldClosed
		legacyRuntimeLock.Unlock()
	})

	entered, release := make(chan struct{}), make(chan struct{})
	if !StartLegacyBootTask(func(ctx context.Context) {
		close(entered)
		<-ctx.Done()
		<-release
	}) {
		t.Fatal("boot task was not admitted")
	}
	<-entered
	deadline, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := closeLegacyBootTasks(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected retryable close timeout, got %v", err)
	}
	if StartLegacyBootTask(func(context.Context) {}) {
		t.Fatal("stopped boot owner admitted late work")
	}
	close(release)
	if err := closeLegacyBootTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
}
