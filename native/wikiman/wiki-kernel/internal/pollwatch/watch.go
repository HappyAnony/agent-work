// Package pollwatch gives the existing polling backend an explicit lifetime.
package pollwatch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/radovskyb/watcher"
	backend "wiki-kernel/internal/pollwatch/backend"
)

type Watch struct {
	backend  *backend.Watcher
	ctx      context.Context
	cancel   context.CancelFunc
	stopping atomic.Bool
	stopOnce sync.Once
	done     chan struct{}
	workers  sync.WaitGroup
}

// Open preserves Add (nonrecursive) and polling event semantics. No directory
// creation or implicit source selection is allowed. The callback Context is
// canceled before backend shutdown and Close joins any callback already active.
// Callbacks must not retain mutable workspace globals in a contextual service.
func Open(parent context.Context, folder string, interval time.Duration, event func(context.Context, watcher.Event), report func(error)) (*Watch, error) {
	if parent == nil || !filepath.IsAbs(folder) || interval <= 0 || event == nil || report == nil {
		return nil, errors.New("invalid_pollwatch_options")
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	backend := backend.New()
	if err := backend.Add(folder); err != nil {
		return nil, err
	}
	// Retain caller identity values, but only the Watch owner controls the
	// long-lived callback cancellation boundary after successful admission.
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	w := &Watch{backend: backend, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	// Register all workers before publishing the owner. The third is the one
	// closer launched by Stop; its slot prevents a concurrent Wait/Add race.
	w.workers.Add(3)
	go func() {
		defer w.workers.Done()
		if err := backend.Start(interval); err != nil {
			report(err)
		}
	}()
	go func() {
		defer w.workers.Done()
		for {
			select {
			case e := <-backend.Event:
				if !w.stopping.Load() {
					func() {
						defer func() {
							if value := recover(); value != nil {
								report(fmt.Errorf("watch callback panic: %v", value))
							}
						}()
						event(w.ctx, watcher.Event{Op: watcher.Op(e.Op), Path: e.Path, OldPath: e.OldPath, FileInfo: e.FileInfo})
					}()
				}
			case err := <-backend.Error:
				if !w.stopping.Load() {
					report(err)
				}
			case <-backend.Closed:
				return
			}
		}
	}()
	go func() { w.workers.Wait(); close(w.done) }()
	backend.Wait() // Stop cannot miss an as-yet-unstarted backend.
	return w, nil
}

// Stop stops callback admission immediately. Keep consuming backend events until
// its polling loop exits, otherwise its unbuffered Event send can block Close.
func (w *Watch) Stop() {
	w.stopOnce.Do(func() {
		w.stopping.Store(true)
		w.cancel()
		go func() { defer w.workers.Done(); w.backend.Close() }()
	})
}
func (w *Watch) Close(ctx context.Context) error {
	w.Stop()
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (w *Watch) Done() <-chan struct{} { return w.done }
