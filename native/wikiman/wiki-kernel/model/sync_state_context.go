// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"time"
	"wiki-kernel/conf"
)

type SyncState struct {
	Synced      int64
	Stat        string
	Enabled     bool
	Mode        int
	HasUser     bool
	Boot        int
	BootMessage string
	Kernels     []*OnlineKernel
	Kernel      string
}

func SyncStateContext(ctx context.Context) (out SyncState, err error) {
	o, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return out, err
	}
	o.owner.m.Lock()
	if o.owner.Sync == nil {
		o.owner.m.Unlock()
		return out, conf.ErrBusinessUnbound
	}
	s := *o.owner.Sync
	out = SyncState{Synced: s.Synced, Stat: s.Stat, Enabled: s.Enabled, Mode: s.Mode, HasUser: o.owner.User != nil, Boot: int(o.owner.syncRuntime.boot.Load()) - 1, Kernel: KernelID}
	if !s.Enabled {
		out.Stat = o.owner.Language(53)
	}
	out.BootMessage = o.owner.Language(17)
	o.owner.m.Unlock()
	out.Kernels = o.owner.syncRuntime.onlineKernels(out.Kernel)
	if err = o.guard(ctx); err != nil {
		return SyncState{}, err
	}
	return out, nil
}

func BootSyncStatusContext(ctx context.Context) (int, error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return -1, e
	}
	return int(o.owner.syncRuntime.boot.Load()) - 1, o.guard(ctx)
}

func ExitSyncStatusContext(ctx context.Context) (int, error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return -1, e
	}
	return int(o.owner.syncRuntime.exit.Load()) - 1, o.guard(ctx)
}

func GetOnlineKernelsContext(ctx context.Context) ([]*OnlineKernel, error) {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	out := o.owner.syncRuntime.onlineKernels(KernelID)
	if e = o.guard(ctx); e != nil {
		return nil, e
	}
	return out, nil
}

func IncSyncContext(ctx context.Context) error {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	o.owner.m.Lock()
	if o.owner.Sync == nil {
		o.owner.m.Unlock()
		return conf.ErrBusinessUnbound
	}
	interval := o.owner.Sync.Interval
	o.owner.m.Unlock()
	o.owner.syncRuntime.same.Store(0)
	o.owner.planSyncAfter(time.Duration(interval) * time.Second)
	return o.guard(ctx)
}
