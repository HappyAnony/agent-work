// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"time"
	"wiki-kernel/internal/notebooks"
)

func NotebookContext(ctx context.Context, id string, opened bool) (*Box, error) {
	value, e := NotebookConfigurationContext(ctx, id)
	if e != nil {
		return nil, e
	}
	if opened && value.Closed {
		return nil, errors.New("notebook_not_open")
	}
	return &Box{ID: id, Name: value.Name, Icon: value.Icon, Sort: value.Sort, SortMode: value.SortMode, Closed: value.Closed}, nil
}
func NotebookInfoContext(ctx context.Context, id string) (*BoxInfo, error) {
	return runWorkspaceMetadataRead(ctx, func(o *workspaceReadOwner, c context.Context) (out *BoxInfo, result error) {
		io, closeIO, e := o.r.openRepairIOChecked(c, false)
		if e != nil {
			return nil, e
		}
		defer func() {
			result = joinRepairCloseError(result, closeIO())
			if result != nil {
				out = nil
			}
		}()
		value, _, e := notebooks.Configuration(c, io.Source, id, ".siyuan")
		if e != nil {
			return nil, e
		}
		if value.Closed {
			return nil, errors.New("notebook_not_open")
		}
		formatter, e := o.base.owner.business.HumanizeTime(o.settings.Lang)
		if e != nil {
			return nil, e
		}
		now := time.Now()
		return notebooks.Information(c, io.Source, id, value.Name, time.Local, func(t time.Time) string { return formatter(t, now) })
	})
}
