// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

// legacyDelayedUIOwner freezes the workspace authority used by compatibility
// entry points. A queued callback may finish only for the owner and roots that
// admitted it; it must never borrow a replacement workspace at dispatch time.
type legacyDelayedUIOwner struct {
	owner                    *AppConf
	workspace, data, working string
	lang                     string
	cloudRegion              int
}

type repoDelayedTask struct {
	action string
	args   []interface{}
}

func captureLegacyDelayedUIOwner(ctx context.Context) (*legacyDelayedUIOwner, error) {
	if ctx == nil {
		return nil, errors.New("delayed_ui_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	o := &legacyDelayedUIOwner{owner: base.owner, workspace: base.workspace, data: base.data, working: util.WorkingDir}
	if o.owner == nil {
		return nil, conf.ErrBusinessUnbound
	}
	if err := o.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		o.lang, o.cloudRegion = v.Lang, v.CloudRegion
		return nil
	}); err != nil {
		return nil, err
	}
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed || !o.current() {
		return nil, errors.New("delayed_ui_owner_closed")
	}
	return o, nil
}

func (o *legacyDelayedUIOwner) current() bool {
	return o != nil && o.owner != nil && o.owner == Conf && o.workspace == util.WorkspaceDir && o.data == util.DataDir && o.working == util.WorkingDir
}

func (o *legacyDelayedUIOwner) language(code int, html bool) string {
	if o == nil || o.owner == nil || o.owner.business == nil {
		return ""
	}
	return o.owner.business.Language(o.lang, code, html, o.cloudRegion)
}

func (o *legacyDelayedUIOwner) publish(ctx context.Context, fn func()) bool {
	if o == nil || o.owner == nil || ctx == nil || fn == nil || ctx.Err() != nil {
		return false
	}
	o.owner.repoMu.Lock()
	defer o.owner.repoMu.Unlock()
	if o.owner.repoClosed || ctx.Err() != nil || !o.current() {
		return false
	}
	fn()
	return true
}

func (o *legacyDelayedUIOwner) schedule(ctx context.Context, action string, delay time.Duration, fn func(), identity ...interface{}) error {
	if o == nil || o.owner == nil || ctx == nil || fn == nil {
		return errors.New("delayed_ui_owner_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	candidate := &repoDelayedTask{action: action, args: append([]interface{}(nil), identity...)}
	o.owner.repoMu.Lock()
	if o.owner.repoClosed {
		o.owner.repoMu.Unlock()
		return errors.New("delayed_ui_owner_closed")
	}
	if !o.current() {
		o.owner.repoMu.Unlock()
		return errors.New("delayed_ui_owner_changed")
	}
	if delayedUIActionUnique(action) {
		for _, pending := range o.owner.repoDelayed {
			if pending.action == candidate.action && reflect.DeepEqual(pending.args, candidate.args) {
				o.owner.repoMu.Unlock()
				return nil
			}
		}
	}
	if o.owner.repoTasks == nil {
		o.owner.repoTasks = taskgroup.Open()
	}
	o.owner.repoDelayed = append(o.owner.repoDelayed, candidate)
	accepted := o.owner.repoTasks.Go(func(life context.Context) {
		defer logging.Recover()
		defer o.owner.removeRepoDelayed(candidate)
		runCtx, cancel := context.WithCancel(life)
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
		defer cancel()
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-runCtx.Done():
			return
		case <-timer.C:
		}
		o.publish(runCtx, fn)
	})
	if !accepted {
		o.owner.removeRepoDelayedLocked(candidate)
		o.owner.repoMu.Unlock()
		return errors.New("delayed_ui_owner_closed")
	}
	o.owner.repoMu.Unlock()
	return nil
}

func delayedUIActionUnique(action string) bool {
	switch action {
	case task.ReloadAttributeView, task.ReloadProtyle, task.ReloadTag, task.ReloadFiletree:
		return true
	default:
		return false
	}
}

func (c *AppConf) removeRepoDelayed(target *repoDelayedTask) {
	c.repoMu.Lock()
	defer c.repoMu.Unlock()
	c.removeRepoDelayedLocked(target)
}

func (c *AppConf) removeRepoDelayedLocked(target *repoDelayedTask) {
	for i, pending := range c.repoDelayed {
		if pending == target {
			c.repoDelayed = append(c.repoDelayed[:i], c.repoDelayed[i+1:]...)
			return
		}
	}
}

func ScheduleLegacyUIReloadContext(ctx context.Context) bool {
	owner, err := captureLegacyDelayedUIOwner(ctx)
	if err != nil {
		return false
	}
	if err = owner.schedule(context.WithoutCancel(ctx), "reloadUI", 200*time.Millisecond, util.ReloadUI); err != nil {
		return false
	}
	return true
}

func scheduleLegacyDelayedUI(action string, delay time.Duration, fn func(), identity ...interface{}) bool {
	o, err := captureLegacyDelayedUIOwner(context.Background())
	if err != nil {
		return false
	}
	return o.schedule(context.Background(), action, delay, fn, identity...) == nil
}
