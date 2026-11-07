// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/repoview"
)

func (l *Lease) NativeReadRepository(ctx context.Context, q repoview.Request) (repoview.Result, error) {
	var service *cloudrepo.Service
	if e := l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		service = l.workspace.resources.cloudRepo
		if service == nil {
			return cloudrepo.ErrDependency
		}
		return nil
	}); e != nil {
		return repoview.Result{}, e
	}
	out := make(chan repoview.Result, 1)
	task, e := l.StartTask(ctx, func(c context.Context, pin *Lease) error {
		v, e := service.ReadRepository(c, q)
		if e == nil {
			out <- v
		}
		return e
	})
	if e != nil {
		return repoview.Result{}, e
	}
	if e = task.Wait(context.WithoutCancel(ctx)); e != nil {
		return repoview.Result{}, e
	}
	return <-out, nil
}
