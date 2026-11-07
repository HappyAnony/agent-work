package txqueue

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestIndependentOwnersAndOrderedDrain(t *testing.T) {
	ctx := testContext(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var got []int
	a := Open(2, func(n int) {
		if n == 1 {
			close(entered)
			<-release
		}
		got = append(got, n)
	})
	b := Open(2, func(n int) {})
	if err := a.Submit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := a.Submit(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := b.Submit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(ctx); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := a.Close(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := a.Submit(ctx, 3); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	close(release)
	if err := a.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatal(got)
	}
	if err := a.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestBackpressureAndCancellation(t *testing.T) {
	ctx := testContext(t)
	entered, release := make(chan struct{}), make(chan struct{})
	q := Open(1, func(n int) {
		if n == 1 {
			close(entered)
			<-release
		}
	})
	if err := q.Submit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := q.Submit(ctx, 2); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := q.Submit(cancelled, 3); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := q.Drain(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	if err := q.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	// A drain is not a close: the same owner can accept another operation.
	if err := q.Submit(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if err := q.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPanicDoesNotOrphanDrain(t *testing.T) {
	ctx := testContext(t)
	var got []int
	q := Open(3, func(n int) {
		if n == 1 {
			panic("synthetic")
		}
		got = append(got, n)
	})
	if err := q.Submit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := q.Submit(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if err := q.Close(ctx); !errors.Is(err, ErrPanicked) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []int{2}) {
		t.Fatal(got)
	}
}

func TestConcurrentSubmitClose(t *testing.T) {
	ctx := testContext(t)
	for range 20 {
		q := Open(1, func(int) {})
		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 10 {
					if err := q.Submit(ctx, 1); err != nil && !errors.Is(err, ErrClosed) {
						t.Error(err)
					}
				}
			}()
		}
		if err := q.Close(ctx); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
	}
}
