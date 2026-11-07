package model

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type fsnotifyOwner struct {
	watcher  *fsnotify.Watcher
	cancel   context.CancelFunc
	done     chan struct{}
	resultMu sync.Mutex
	runErr   error
	stopOnce sync.Once
	stopErr  error
}

func openFSNotifyOwner(folder string, run func(context.Context, *fsnotify.Watcher) error) (*fsnotifyOwner, error) {
	if run == nil {
		return nil, errors.New("fsnotify_owner_unbound")
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err = watcher.Add(folder); err != nil {
		_ = watcher.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	owner := &fsnotifyOwner{watcher: watcher, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(owner.done)
		defer func() {
			if value := recover(); value != nil {
				owner.setRunError(fmt.Errorf("fsnotify callback panic: %v", value))
			}
		}()
		owner.setRunError(run(ctx, watcher))
	}()
	return owner, nil
}

func (o *fsnotifyOwner) setRunError(err error) {
	if err == nil {
		return
	}
	o.resultMu.Lock()
	o.runErr = errors.Join(o.runErr, err)
	o.resultMu.Unlock()
}

func (o *fsnotifyOwner) result() error {
	o.resultMu.Lock()
	defer o.resultMu.Unlock()
	return errors.Join(o.stopErr, o.runErr)
}

func (o *fsnotifyOwner) Stop() error {
	if o == nil {
		return nil
	}
	o.stopOnce.Do(func() {
		o.cancel()
		o.stopErr = o.watcher.Close()
	})
	return o.stopErr
}

func (o *fsnotifyOwner) Close(ctx context.Context) error {
	stopErr := o.Stop()
	select {
	case <-o.done:
		return o.result()
	case <-ctx.Done():
		return errors.Join(stopErr, ctx.Err())
	}
}

func (o *fsnotifyOwner) Done() <-chan struct{} { return o.done }
