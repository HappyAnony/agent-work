package eventscope

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLocalBusUnsubscribeAndDrainRealCallbacks(t *testing.T) {
	var bus LocalBus[string]
	ctx := context.Background()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	s, err := bus.Subscribe(func(value string) {
		if value != "A" {
			t.Error(value)
		}
		close(entered)
		<-release
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { done <- bus.Publish("A") }()
	<-entered
	deadline, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err = s.Close(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err = bus.Close(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("retired active callback not joined", err)
	}
	if _, err = bus.Subscribe(func(string) {}); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = bus.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err = bus.Publish("late"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestLocalBusPanicReleasesSubscription(t *testing.T) {
	var bus LocalBus[int]
	s, err := bus.Subscribe(func(int) { panic("synthetic") })
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic swallowed")
			}
		}()
		_ = bus.Publish(1)
	}()
	if err = s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = bus.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLocalBusContextCancellationUnsubscribesAndJoinsCallback(t *testing.T) {
	var bus LocalBus[string]
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	var calls int
	s, err := bus.SubscribeContext(ctx, func(string) {
		calls++
		close(entered)
		<-release
	})
	if err != nil {
		t.Fatal(err)
	}
	published := make(chan struct{})
	go func() { _ = bus.Publish("first"); close(published) }()
	<-entered
	cancel()
	deadline, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if err = s.Close(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("active callback reported drained", err)
	}
	if err = bus.Publish("late"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("cancelled subscription admitted a late callback", calls)
	}
	close(release)
	<-published
	if err = s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = bus.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	var fresh LocalBus[string]
	cancelled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err = fresh.SubscribeContext(cancelled, func(string) {}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
