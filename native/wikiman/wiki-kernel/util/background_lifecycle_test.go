package util

import (
	"context"
	"testing"
	"time"
)

func TestBackgroundLoopsRespectCancelledOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		CheckFileSysStatusContext(ctx)
		RefreshRhyResultJobContext(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background callback ignored cancelled owner")
	}
	if waitFileSysCheck(ctx, time.Hour) {
		t.Fatal("cancelled sleep succeeded")
	}
}
