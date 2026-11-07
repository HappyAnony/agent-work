// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudrepo

import (
	"context"
	"wiki-kernel/conf"
	"wiki-kernel/internal/repoview"
)

func (s *Service) ReadRepository(ctx context.Context, q repoview.Request) (out repoview.Result, err error) {
	if ctx == nil {
		return out, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if s.o.ReadRepository == nil {
		return out, ErrDependency
	}
	switch q.Kind {
	case "file", "preview", "snapshot-file":
		if !ValidSnapshotID(q.ID) {
			return out, ErrSnapshotID
		}
	case "diff":
		if !ValidSnapshotID(q.Left) || !ValidSnapshotID(q.Right) {
			return out, ErrSnapshotID
		}
	case "logs":
		if q.Page < 1 {
			return out, ErrOperation
		}
	case "tags":
	default:
		return out, ErrOperation
	}
	err = s.maintenance(ctx, func(c context.Context, v conf.AppData) error {
		var e error
		out, e = s.o.ReadRepository(c, v, q)
		return e
	})
	if err != nil {
		return repoview.Result{}, err
	}
	return
}
