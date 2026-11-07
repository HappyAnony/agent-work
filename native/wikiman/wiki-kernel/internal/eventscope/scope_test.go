package eventscope

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestCloseRemovesCallbacksAndDrainsActualLifetime(t *testing.T) {
	var relay func(string)
	registrations := 0
	b := NewBridge(func(_ string, fn interface{}) error { registrations++; relay = fn.(func(string)); return nil })
	a := b.Open()
	entered, release := make(chan struct{}), make(chan struct{})
	a.Subscribe("topic", func(s string) { close(entered); <-release })
	finished := make(chan struct{})
	go func() { relay("a"); close(finished) }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Close(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	if len(b.routes["topic"].handlers) != 0 {
		t.Fatal("callback references retained")
	}
	if err := a.Subscribe("topic", func(string) {}); err != ErrClosed {
		t.Fatal(err)
	}
	// Late publication must not enter the removed callback or block on release.
	relay("late")
	close(release)
	<-finished
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	c := b.Open()
	if err := c.Subscribe("topic", func(string) { calls.Add(1) }); err != nil {
		t.Fatal(err)
	}
	relay("new")
	c.Close(context.Background())
	relay("closed")
	if calls.Load() != 1 || registrations != 1 {
		t.Fatal(calls.Load(), registrations)
	}
}

func TestIndependentScopesRegistrationFailureAndSignatures(t *testing.T) {
	errRegistration := errors.New("register failed")
	var relay func()
	fail := true
	b := NewBridge(func(_ string, fn interface{}) error {
		if fail {
			return errRegistration
		}
		relay = fn.(func())
		return nil
	})
	a, c := b.Open(), b.Open()
	if err := a.Subscribe("t", func() {}); err != errRegistration {
		t.Fatal(err)
	}
	if len(b.routes) != 0 {
		t.Fatal("failed route retained")
	}
	fail = false
	var aCalls, cCalls int
	a.Subscribe("t", func() { aCalls++ })
	c.Subscribe("t", func() { cCalls++ })
	if err := c.Subscribe("t", func(string) {}); err == nil {
		t.Fatal("signature accepted")
	}
	if err := c.Subscribe("invalid", 1); err == nil {
		t.Fatal("nonfunction accepted")
	}
	a.Close(context.Background())
	relay()
	if aCalls != 0 || cCalls != 1 {
		t.Fatal(aCalls, cCalls)
	}
	c.Close(context.Background())
}

func TestPanicStillReleasesPin(t *testing.T) {
	var relay func()
	b := NewBridge(func(_ string, fn interface{}) error { relay = fn.(func()); return nil })
	s := b.Open()
	s.Subscribe("panic", func() { panic("expected") })
	func() { defer func() { _ = recover() }(); relay() }()
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
