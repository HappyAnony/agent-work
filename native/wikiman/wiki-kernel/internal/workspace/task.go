package workspace

import (
	"context"
	"errors"
	"time"
)

type Task struct {
	done   chan struct{}
	cancel context.CancelFunc
	err    error
}

func (t *Task) Done() <-chan struct{} { return t.done }
func (t *Task) Cancel()               { t.cancel() }
func (t *Task) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.done:
		return t.err
	}
}

// StartTask is the only owned background dispatch primitive. A bounded task
// retains its workspace before starting and releases even on error or panic.
// Drain never force-closes a task that ignores cancellation; its deadline wins.
func (l *Lease) StartTask(ctx context.Context, fn func(context.Context, *Lease) error) (*Task, error) {
	if fn == nil {
		return nil, ErrIdentity
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m := l.manager
	m.mu.Lock()
	if l.released {
		m.mu.Unlock()
		return nil, ErrStale
	}
	if m.draining || l.workspace.store.draining || l.workspace.store.mutating || l.workspace.store.recoveryRequired {
		m.mu.Unlock()
		return nil, ErrClosed
	}
	if l.workspace.tasks >= m.options.MaxTasksPerWorkspace {
		m.mu.Unlock()
		return nil, ErrBudget
	}
	l.workspace.refs += 2 // An unexposed owner prevents early callback Release from unpinning the task.
	l.workspace.tasks++
	pin := &Lease{manager: m, workspace: l.workspace}
	owner := &Lease{manager: m, workspace: l.workspace}
	ctx, cancel := context.WithCancel(ctx)
	task := &Task{done: make(chan struct{}), cancel: cancel}
	if l.workspace.taskOwners == nil {
		l.workspace.taskOwners = make(map[*Task]context.CancelFunc)
	}
	l.workspace.taskOwners[task] = cancel
	m.mu.Unlock()
	go func() {
		defer func() {
			if recover() != nil {
				task.err = errors.New("workspace_task_panicked")
			}
			cancel()
			m.mu.Lock()
			l.workspace.tasks--
			delete(l.workspace.taskOwners, task)
			m.mu.Unlock()
			pin.Release()
			owner.Release()
			close(task.done)
		}()
		task.err = fn(ctx, pin)
	}()
	return task, nil
}

// Called only with manager.mu held. Cancellation is advisory; leases/resources
// remain pinned until the callback really returns, including ignored cancels.
func (w *WorkspaceContext) cancelTasks() {
	for _, cancel := range w.taskOwners {
		cancel()
	}
}

// StartPeriodicTask binds repeated work to its creation identity/generation.
// DrainStore cancels only that store's tasks; no global scheduler is consulted.
func (l *Lease) StartPeriodicTask(ctx context.Context, interval time.Duration, fn func(context.Context, *Lease) error) (*Task, error) {
	if interval <= 0 || fn == nil {
		return nil, ErrIdentity
	}
	return l.StartTask(ctx, func(ctx context.Context, lease *Lease) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := fn(ctx, lease); err != nil {
					return err
				}
			}
		}
	})
}
