// SPDX-License-Identifier: AGPL-3.0-or-later
package task

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/88250/gulu"
	"wiki-kernel/util"
)

var ErrClosed = errors.New("legacy_task_queue_closed")
var ErrHandler = errors.New("context_task_handler_failed")
var ErrDuplicate = errors.New("context_task_duplicate")

// ContextTask retains actual handler lifetime. Pending cancellation completes
// immediately; cancellation of a running handler is only a request to stop.
type ContextTask struct {
	mu     sync.Mutex
	phase  int
	parent context.Context
	cancel context.CancelFunc
	stop   func() bool
	fn     func(context.Context) error
	done   chan struct{}
	err    error
}

func AppendContext(ctx context.Context, action string, fn func(context.Context) error) (*ContextTask, error) {
	return appendContext(ctx, action, false, false, 0, fn)
}

// AppendUniqueContext preserves the original unique-action queue contract for
// Context-aware handlers. A duplicate does not borrow or cancel the first
// request's lifetime; callers may treat ErrDuplicate as an already accepted
// operation.
func AppendUniqueContext(ctx context.Context, action string, fn func(context.Context) error) (*ContextTask, error) {
	return appendContext(ctx, action, true, false, 0, fn)
}

// AppendAsyncContextWithDelay is the context-aware counterpart of
// AppendAsyncTaskWithDelay. It retains the original action/argument based
// coalescing rules while making pending and running work cancellable and
// joinable by the legacy task owner.
func AppendAsyncContextWithDelay(ctx context.Context, action string, delay time.Duration, fn func(context.Context) error, args ...interface{}) (*ContextTask, error) {
	return appendContext(ctx, action, false, true, delay, fn, args...)
}

func appendContext(ctx context.Context, action string, unique, async bool, delay time.Duration, fn func(context.Context) error, args ...interface{}) (*ContextTask, error) {
	if fn == nil {
		return nil, ErrHandler
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	taskOwnerMu.Lock()
	defer taskOwnerMu.Unlock()
	if !acceptingTasks || taskOwner == nil || util.IsExiting.Load() {
		return nil, ErrClosed
	}
	candidate := &Task{Action: action, Args: args}
	if unique {
		for _, queued := range getCurrentTasks() {
			if queued.Action == action {
				return nil, ErrDuplicate
			}
		}
	} else if gulu.Str.Contains(action, uniqueActions) && containTask(candidate, getCurrentTasks()) {
		return nil, ErrDuplicate
	}
	h := &ContextTask{parent: ctx, fn: fn, done: make(chan struct{})}
	h.mu.Lock()
	h.stop = context.AfterFunc(ctx, h.Cancel)
	h.mu.Unlock()
	queueLock.Lock()
	taskQueue = append(taskQueue, &Task{Action: action, Args: args, Created: time.Now(), Async: async, Delay: delay, Timeout: 24 * time.Hour, Owned: h})
	queueLock.Unlock()
	return h, nil
}
func (h *ContextTask) Cancel() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.phase == 0 {
		h.phase = 2
		h.fn = nil
		h.err = context.Canceled
		close(h.done)
		if h.stop != nil {
			h.stop()
		}
	} else if h.phase == 1 && h.cancel != nil {
		h.cancel()
	}
}
func (h *ContextTask) run(life context.Context) {
	h.mu.Lock()
	if h.phase != 0 {
		h.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(life)
	h.cancel = cancel
	h.phase = 1
	h.mu.Unlock()
	stop := context.AfterFunc(h.parent, cancel)
	var err error
	defer func() {
		if recover() != nil {
			err = ErrHandler
		}
		stop()
		cancel()
		h.mu.Lock()
		h.err = err
		h.phase = 2
		close(h.done)
		h.mu.Unlock()
		h.stop()
	}()
	if err = ctx.Err(); err != nil {
		return
	}
	if err = h.parent.Err(); err != nil {
		return
	}
	err = h.fn(ctx)
}
func (h *ContextTask) Wait(ctx context.Context) error {
	select {
	case <-h.done:
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (h *ContextTask) Join(ctx context.Context) error {
	select {
	case <-h.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func Accepting() bool {
	taskOwnerMu.Lock()
	defer taskOwnerMu.Unlock()
	return acceptingTasks && !util.IsExiting.Load()
}
