// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/notebooks"
)

func CreateBoxContext(ctx context.Context, name string) (string, error) {
	return runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (out string, result error) {
		r, e := captureIndexRepair(c)
		if e != nil {
			return "", e
		}
		if r.owner != o.base.owner || r.data != o.base.data {
			return "", conf.ErrBusinessUnbound
		}
		result = r.owner.notebookGate().with(c, func() (err error) {
			if e := r.guard(c); e != nil {
				return e
			}
			if o.settings.FileTree == nil || o.settings.Sync == nil {
				return conf.ErrBusinessUnbound
			}
			if e := FlushTxQueueContext(c); e != nil {
				return e
			}
			// Preserve the original notebook repair policy, but propagate failures.
			if _, e := ListNotebooksContext(c); e != nil {
				return e
			}
			io, closeIO, e := r.openRepairIOChecked(c, true)
			if e != nil {
				return e
			}
			defer func() { err = joinRepairCloseError(err, closeIO()) }()
			untitled := r.owner.business.Language(o.settings.Lang, 105, false, o.settings.CloudRegion)
			plan, e := notebooks.PrepareCreate(c, io.Source, ".siyuan", name, untitled, o.settings.FileTree.Sort, ast.NewNodeID)
			if errors.Is(e, notebooks.ErrNameLength) {
				return errors.New(r.language(106))
			}
			if e != nil {
				return e
			}
			for _, change := range plan.Changes {
				if e = io.Source.Rewrite(c, change.Path, change.Before, change.After); e != nil {
					return e
				}
			}
			r.owner.syncRuntime.same.Store(0)
			r.owner.planSyncAfter(time.Duration(o.settings.Sync.Interval) * time.Second)
			out = plan.CreatedID
			return c.Err()
		})
		return out, result
	})
}
