// Package txqueue owns the lifecycle of a serial transaction executor. Opening
// an instance, rather than importing a package, starts its only worker.
package txqueue

import (
	"context"
	"errors"
	"sync"
)

var ErrClosed = errors.New("transaction_queue_closed")
var ErrPanicked = errors.New("transaction_queue_panicked")

type Queue[T any] struct {
	mu       sync.Mutex
	changed  chan struct{}
	done     chan struct{}
	items    []T
	capacity int
	active   bool
	closing  bool
	err      error
	run      func(T)
}

// Open starts one independently owned worker. No source, transport or global
// workspace is selected here; the callback must already own its dependencies.
func Open[T any](capacity int, run func(T)) *Queue[T] {
	if capacity < 1 || run == nil {
		panic("invalid transaction queue")
	}
	q := &Queue[T]{capacity: capacity, run: run, changed: make(chan struct{}), done: make(chan struct{})}
	go q.work()
	return q
}

func (q *Queue[T]) notify() { close(q.changed); q.changed = make(chan struct{}) }

// Submit confirms admission only, not persistence. Callers needing a receipt
// must obtain it from the transaction service, not from this transport queue.
func (q *Queue[T]) Submit(ctx context.Context, item T) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if q.closing {
			return ErrClosed
		}
		if len(q.items) < q.capacity {
			q.items = append(q.items, item)
			q.notify()
			return nil
		}
		changed := q.changed
		q.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-changed:
		}
		q.mu.Lock()
	}
}

// Drain waits for admitted work without closing admission. Lifecycle owners
// must stop new producers first or use Close to atomically stop admission.
func (q *Queue[T]) Drain(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) != 0 || q.active {
		changed := q.changed
		q.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-changed:
		}
		q.mu.Lock()
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return q.err
}

// Close rejects new submissions, drains accepted work and joins the worker.
// A timeout never force-closes dependencies of a running transaction. Retrying
// Close is safe. An abandoned queue does not silently reopen itself.
func (q *Queue[T]) Close(ctx context.Context) error {
	q.mu.Lock()
	q.closing = true
	q.notify()
	q.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-q.done:
		q.mu.Lock()
		defer q.mu.Unlock()
		return q.err
	}
}

func (q *Queue[T]) work() {
	defer close(q.done)
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if len(q.items) == 0 {
			if q.closing {
				return
			}
			changed := q.changed
			q.mu.Unlock()
			<-changed
			q.mu.Lock()
			continue
		}
		item := q.items[0]
		var zero T
		q.items[0] = zero
		q.items = q.items[1:]
		q.active = true
		q.notify()
		q.mu.Unlock()
		panicked := true
		func() {
			defer func() { _ = recover() }()
			q.run(item)
			panicked = false
		}()
		q.mu.Lock()
		if panicked {
			q.err = ErrPanicked
		}
		q.active = false
		q.notify()
	}
}
