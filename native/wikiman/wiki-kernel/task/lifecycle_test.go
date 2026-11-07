package task

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestLegacyTaskTimeoutDoesNotReleaseHandler(t *testing.T) {
	OpenLegacyTasks()
	entered, release := make(chan struct{}), make(chan struct{})
	finished := make(chan struct{})
	go func() {
		execTask(&Task{Action: "test-owned-timeout", Async: true, Timeout: time.Millisecond, Handler: reflect.ValueOf(func() { close(entered); <-release })})
		close(finished)
	}()
	<-entered
	<-finished // original timeout returned, real handler is still alive
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CloseLegacyTasks(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	AppendTask("late", func() { t.Error("late task") })
	if len(getCurrentTasks()) != 0 {
		t.Fatal("closed queue admitted task")
	}
	close(release)
	if err := CloseLegacyTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
}
