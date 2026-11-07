// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/sql"
)

// The compatibility HTTP adapter pins one workspace for the complete request,
// including foreground effects, streaming responses and deferred cleanup. Its
// Context identity also prevents nested adapters from selecting a replacement.
// Startup probes may precede Conf; business adapters still reject unbound IO.
func RunLegacyRequestContext(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil || fn == nil {
		return conf.ErrBusinessUnbound
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if Conf == nil {
		if ctx.Value(legacyAdmissionKey{}) != nil {
			return conf.ErrBusinessUnbound
		}
		return fn(ctx)
	}
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	return base.run(ctx, func(c, _ context.Context) error { return fn(c) })
}

// Callbacks must not reenter this owner's setting gate.
func WithLegacyConfigurationSnapshotContext(ctx context.Context, fn func(context.Context, conf.AppData) error) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	if fn == nil {
		return conf.ErrBusinessUnbound
	}
	return base.run(ctx, func(c, _ context.Context) error {
		return base.owner.WithSettingSnapshot(c, func(v conf.AppData) error {
			if e := base.guard(c); e != nil {
				return e
			}
			return fn(c, v)
		})
	})
}

func FlushForegroundTransactionsContext(ctx context.Context) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	queue := sql.CaptureLegacyMainQueue()
	return base.run(ctx, func(c, _ context.Context) error {
		if e := FlushTxQueueContext(c); e != nil {
			return e
		}
		return queue.Flush(c)
	})
}
