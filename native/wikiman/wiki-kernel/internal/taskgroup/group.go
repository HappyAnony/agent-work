// Package taskgroup tracks actual callback lifetime, not a caller's timeout.
package taskgroup

import (
	"context"
	"sync"
	"time"
)

type Group struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	stopped bool
	active  int
	done    chan struct{}
}

func Open() *Group {
	ctx, cancel := context.WithCancel(context.Background())
	return &Group{ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

// Go registers before launching; admission and Stop are serialized. A callback
// ignoring cancellation remains owned until it really returns.
func (g *Group) Go(fn func(context.Context)) bool {
	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		return false
	}
	g.active++
	g.mu.Unlock()
	go func() {
		defer func() {
			g.mu.Lock()
			g.active--
			if g.stopped && g.active == 0 {
				close(g.done)
			}
			g.mu.Unlock()
		}()
		fn(g.ctx)
	}()
	return true
}

func (g *Group) Every(interval, delay time.Duration, fn func()) bool {
	if fn == nil {
		return false
	}
	return g.EveryContext(interval, delay, func(context.Context) { fn() })
}

func (g *Group) EveryContext(interval, delay time.Duration, fn func(context.Context)) bool {
	if interval <= 0 || delay < 0 || fn == nil {
		return false
	}
	return g.Go(func(ctx context.Context) {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		fn(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				fn(ctx)
			}
		}
	})
}

func (g *Group) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return
	}
	g.stopped = true
	g.cancel()
	if g.active == 0 {
		close(g.done)
	}
}

// Wait does not stop admission implicitly. Close is the normal shutdown entry.
func (g *Group) Wait(ctx context.Context) error {
	select {
	case <-g.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (g *Group) Close(ctx context.Context) error { g.Stop(); return g.Wait(ctx) }
func (g *Group) Done() <-chan struct{}           { return g.done }
