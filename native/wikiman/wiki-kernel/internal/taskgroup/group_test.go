package taskgroup

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClosePinsActualCallbackAndRetries(t *testing.T) {
	g := Open()
	entered, release := make(chan struct{}), make(chan struct{})
	if !g.Go(func(context.Context) { close(entered); <-release }) {
		t.Fatal("admission")
	}
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Close(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	if g.Go(func(context.Context) { t.Error("late callback") }) {
		t.Fatal("late admission")
	}
	select {
	case <-g.Done():
		t.Fatal("closed while callback is live")
	default:
	}
	close(release)
	if err := g.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStopBeforeScheduledStartAndIndependentOwners(t *testing.T) {
	a, b := Open(), Open()
	var aCalls, bCalls atomic.Int32
	a.Every(time.Millisecond, time.Hour, func() { aCalls.Add(1) })
	a.Close(context.Background())
	ready := make(chan struct{})
	b.Every(time.Hour, 0, func() { bCalls.Add(1); close(ready) })
	<-ready
	b.Close(context.Background())
	if aCalls.Load() != 0 || bCalls.Load() != 1 {
		t.Fatal(aCalls.Load(), bCalls.Load())
	}
}

func TestConcurrentAdmissionAndStop(t *testing.T) {
	for i := 0; i < 50; i++ {
		g := Open()
		var wg sync.WaitGroup
		for j := 0; j < 20; j++ {
			wg.Add(1)
			go func() { defer wg.Done(); g.Go(func(context.Context) {}) }()
		}
		g.Stop()
		wg.Wait()
		if err := g.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}
