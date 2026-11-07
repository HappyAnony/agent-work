// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"time"

	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

// startRepoBackground admits compatibility work to one AppConf. Workspace
// close stops admission and joins every callback that was already accepted.
func (c *AppConf) startRepoBackground(fn func(context.Context)) bool {
	if c == nil || fn == nil {
		return false
	}
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	if c.repoClosed || c != Conf {
		return false
	}
	if c.repoTasks == nil {
		c.repoTasks = taskgroup.Open()
	}
	return c.repoTasks.Go(fn)
}

type joinableWorkspaceWatcher interface {
	Close(context.Context) error
	Done() <-chan struct{}
}

type workspaceWatcherOwner struct {
	owner                          *AppConf
	workspace, data, temp, working string
}

func captureWorkspaceWatcherOwner(ctx context.Context) (*workspaceWatcherOwner, error) {
	if ctx == nil {
		return nil, errors.New("workspace_watcher_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o := &workspaceWatcherOwner{owner: Conf, workspace: util.WorkspaceDir, data: util.DataDir, temp: util.TempDir, working: util.WorkingDir}
	if o.owner == nil {
		return nil, errors.New("workspace_watcher_owner_unbound")
	}
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed || !o.current() {
		return nil, errors.New("workspace_watcher_owner_closed")
	}
	return o, nil
}

func (o *workspaceWatcherOwner) current() bool {
	return o != nil && o.owner != nil && o.owner == Conf && o.workspace == util.WorkspaceDir && o.data == util.DataDir && o.temp == util.TempDir && o.working == util.WorkingDir
}

func (o *workspaceWatcherOwner) guard(ctx context.Context) error {
	if o == nil || o.owner == nil || ctx == nil {
		return errors.New("workspace_watcher_owner_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed || !o.current() {
		return errors.New("workspace_watcher_owner_changed")
	}
	return ctx.Err()
}

func (o *workspaceWatcherOwner) publish(ctx context.Context, fn func()) bool {
	if o == nil || o.owner == nil || ctx == nil || fn == nil || ctx.Err() != nil {
		return false
	}
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed || !o.current() || ctx.Err() != nil {
		return false
	}
	fn()
	return true
}

// ownWorkspaceWatcher ties a platform watcher to the same task group that is
// drained before process-wide paths and databases are replaced.
func (o *workspaceWatcherOwner) ownWorkspaceWatcher(w joinableWorkspaceWatcher) bool {
	if o == nil || o.owner == nil || w == nil {
		return false
	}
	return o.owner.startRepoBackground(func(life context.Context) {
		select {
		case <-life.Done():
			_ = w.Close(context.Background())
		case <-w.Done():
		}
	})
}

func ScheduleUserGuideOpenContext(ctx context.Context, guideStartID, app string) bool {
	if ctx == nil || guideStartID == "" || ctx.Err() != nil {
		return false
	}
	o, err := captureLegacyDelayedUIOwner(ctx)
	if err != nil || ctx.Err() != nil {
		return false
	}
	return o.owner.startRepoBackground(func(life context.Context) {
		for i := 0; i < 70; i++ {
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-life.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if !o.current() {
				return
			}
			if treenode.ExistBlockTree(guideStartID) {
				o.publish(life, func() {
					util.BroadcastByTypeAndApp("main", app, "openFileById", 0, "", map[string]interface{}{"id": guideStartID})
				})
				return
			}
		}
	})
}
