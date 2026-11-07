// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"sync"
	"testing"
	"wiki-kernel/conf"
)

func TestSyncRuntimePeerOwnershipAndDetachedValues(t *testing.T) {
	a, b := &AppConf{}, &AppConf{}
	values := []*OnlineKernel{{ID: "same", Hostname: "owner-a"}, {ID: "self"}}
	a.syncRuntime.replaceKernels(values)
	values[0].Hostname = "changed"
	got := a.syncRuntime.onlineKernels("self")
	if len(got) != 1 || got[0].Hostname != "owner-a" || len(b.syncRuntime.onlineKernels("")) != 0 {
		t.Fatal(got)
	}
	got[0].Hostname = "returned-alias"
	if a.syncRuntime.onlineKernels("self")[0].Hostname != "owner-a" {
		t.Fatal("peer alias escaped")
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				a.syncRuntime.replaceKernels(values)
				_ = a.syncRuntime.onlineKernels("self")
			}
		}()
	}
	wg.Wait()
	a.syncRuntime.replaceKernels(nil)
	if len(a.syncRuntime.onlineKernels("")) != 0 {
		t.Fatal("reset retained peers")
	}
}

func TestSyncStateContextCancellationAndCapturedSettings(t *testing.T) {
	old := Conf
	owner := &AppConf{AppData: conf.AppData{Sync: conf.NewSync()}}
	Conf = owner
	defer func() { Conf = old }()
	owner.Sync.Enabled = true
	owner.Sync.Stat = "captured"
	owner.Sync.Synced = 42
	owner.syncRuntime.boot.Store(2)
	owner.syncRuntime.replaceKernels([]*OnlineKernel{{ID: KernelID}, {ID: "peer", Hostname: "captured"}})
	out, e := SyncStateContext(context.Background())
	if e != nil || out.Stat != "captured" || out.Synced != 42 || out.Boot != 1 || len(out.Kernels) != 1 {
		t.Fatal(out, e)
	}
	if e = IncSyncContext(context.Background()); e != nil {
		t.Fatal(e)
	}
	if owner.syncRuntime.plan.IsZero() {
		t.Fatal("sync not scheduled")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out, e = SyncStateContext(ctx); !errors.Is(e, context.Canceled) || out.Kernels != nil {
		t.Fatal(out, e)
	}
	if e = IncSyncContext(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	Conf = &AppConf{AppData: conf.AppData{Sync: conf.NewSync()}}
	if peers, e := GetOnlineKernelsContext(context.Background()); e != nil || len(peers) != 0 {
		t.Fatal(peers, e)
	}
}
