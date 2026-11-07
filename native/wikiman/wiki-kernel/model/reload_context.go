// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"time"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

func scheduleReloadContext(ctx context.Context, action string, fn func(), identity ...interface{}) error {
	o, e := captureLegacyDelayedUIOwner(ctx)
	if e != nil {
		return e
	}
	return o.schedule(context.WithoutCancel(ctx), action, 200*time.Millisecond, fn, identity...)
}
func ReloadFiletreeContext(ctx context.Context) error {
	return scheduleReloadContext(ctx, task.ReloadFiletree, util.PushReloadFiletree)
}
func ReloadTagContext(ctx context.Context) error {
	return scheduleReloadContext(ctx, task.ReloadTag, util.PushReloadTag)
}
func ReloadAttrViewContext(ctx context.Context, id string) error {
	return scheduleReloadContext(ctx, task.ReloadAttributeView, func() { pushReloadAttrView(id) }, id)
}
func ReloadProtyleContext(ctx context.Context, id string) error {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	return o.run(ctx, func(op, life context.Context) error {
		source, e := captureLegacyAnchorContext(op)
		if e != nil {
			return e
		}
		ui, e := captureLegacyDelayedUIOwner(op)
		if e != nil {
			return e
		}
		env := sourceEventEnvironmentWithOwners(op, life, source, ui, nil)
		env.legacy = false
		return reloadProtyleWithEnvironment(id, env)
	})
}
