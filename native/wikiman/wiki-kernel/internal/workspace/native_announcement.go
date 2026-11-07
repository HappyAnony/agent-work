// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"time"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/indexdb"
)

// Device ledger and network are explicit capabilities. This lookup releases its
// source/resource admission before the task delays, fetches or updates markers.
func (l *Lease) StartNativeAnnouncements(ctx context.Context, delay time.Duration) (*Task, error) {
	if delay < 0 {
		return nil, ErrIdentity
	}
	var service *cloudaccount.Service
	if e := l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		service = l.workspace.resources.cloudAccount
		if service == nil || !service.HasAnnouncements() {
			return cloudaccount.ErrDependency
		}
		return nil
	}); e != nil {
		return nil, e
	}
	return l.StartTask(ctx, func(c context.Context, _ *Lease) error { return service.RunAnnouncements(c, delay) })
}
