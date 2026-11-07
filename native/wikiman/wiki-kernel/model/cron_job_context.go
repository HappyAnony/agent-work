// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/sql"
)

// A periodic callback keeps the workspace selected at scheduler registration.
// Its cancellation and actual completion participate in both scheduler and
// workspace drain, including callbacks which invoke another owned service.
func CaptureWorkspaceJob(ctx context.Context, fn func(context.Context) error) (func(context.Context) error, error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	if fn == nil {
		return nil, conf.ErrBusinessUnbound
	}
	return func(ctx context.Context) error {
		return base.run(ctx, func(c, _ context.Context) error { return fn(c) })
	}, nil
}
func SyncDataJobContext(ctx context.Context) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	owner := base.owner
	owner.syncRuntime.planMu.Lock()
	if owner.syncRuntime.plan.IsZero() {
		owner.syncRuntime.plan = time.Now().Add(fixSyncInterval)
	}
	due := !time.Now().Before(owner.syncRuntime.plan)
	owner.syncRuntime.planMu.Unlock()
	if !due {
		return base.guard(ctx)
	}
	_, e = owner.syncDataContext(ctx, "normal", false)
	return e
}
func RefreshCheckJobContext(ctx context.Context) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	owner := base.owner
	return errors.Join(owner.ScheduleSubscriptionReminder(), owner.ScheduleCloudRefresh(2*time.Minute), owner.ScheduleAnnouncements(), base.guard(ctx))
}
func FlushUpdateRefTextRenameDocJobContext(ctx context.Context) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	queue := sql.CaptureLegacyMainQueue()
	if e = queue.Flush(ctx); e != nil {
		return e
	}
	if e = base.guard(ctx); e != nil {
		return e
	}
	return flushUpdateRefTextRenameDocContext(ctx)
}
