// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Compatibility sync state belongs to the captured AppConf, not the process.
type legacySyncRuntime struct {
	lock          sync.Mutex
	running       atomic.Bool
	same          atomic.Int32
	errors        atomic.Int32
	boot          atomic.Int32 // encoded status + 1; zero means not yet run (-1)
	exit          atomic.Int32
	booting       atomic.Bool
	storages      atomic.Bool
	files         sync.Map
	planMu        sync.Mutex
	plan          time.Time
	noticePending bool // repoMu
	kernelsMu     sync.Mutex
	kernels       []*OnlineKernel
}

func (r *legacySyncRuntime) replaceKernels(values []*OnlineKernel) {
	r.kernelsMu.Lock()
	defer r.kernelsMu.Unlock()
	r.kernels = cloneOnlineKernels(values, "")
}

func cloneOnlineKernels(values []*OnlineKernel, exclude string) []*OnlineKernel {
	out := make([]*OnlineKernel, 0, len(values))
	for _, v := range values {
		if v != nil && (exclude == "" || v.ID != exclude) {
			copy := *v
			out = append(out, &copy)
		}
	}
	return out
}

func (r *legacySyncRuntime) onlineKernels(exclude string) []*OnlineKernel {
	r.kernelsMu.Lock()
	defer r.kernelsMu.Unlock()
	return cloneOnlineKernels(r.kernels, exclude)
}

func BootSyncStatus() int { v, _ := BootSyncStatusContext(context.Background()); return v }
func ExitSyncStatus() int { v, _ := ExitSyncStatusContext(context.Background()); return v }
