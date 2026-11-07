// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/workspacestat"
	"wiki-kernel/util"
)

// CaptureStatJob is called when the scheduler is installed. A later callback
// retains this workspace and never captures whichever Conf happens to be active.
func CaptureStatJob(ctx context.Context) (func(context.Context) error, error) {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return nil, e
	}
	r, e := captureIndexRepair(ctx)
	if e != nil {
		return nil, e
	}
	r.queryOnly = true
	container := util.Container
	return func(ctx context.Context) error {
		return base.run(ctx, func(c, life context.Context) (err error) {
			if e := r.guard(c); e != nil {
				return e
			}
			io, closeIO, e := r.openRepairIOChecked(c, false)
			if e != nil {
				return e
			}
			defer func() { err = joinRepairCloseError(err, closeIO()) }()
			measured, e := workspacestat.Measure(c, io.Source, func(c context.Context, trees bool) (int, error) {
				filter := indexdb.BlockTreeFilter{Kind: "all"}
				if trees {
					filter = indexdb.BlockTreeFilter{Kind: "type", Value: "d"}
				}
				return r.registry.Count(c, filter)
			})
			if e != nil {
				return e
			}
			if e = closeIO(); e != nil {
				return e
			}
			var language func(int) string
			e = base.owner.WithSettingSnapshot(c, func(v conf.AppData) error {
				if e := base.guard(c); e != nil {
					return e
				}
				language = func(code int) string { return base.owner.business.Language(v.Lang, code, true, v.CloudRegion) }
				base.owner.m.Lock()
				defer base.owner.m.Unlock()
				if base.owner.persistenceReadOnly {
					return ErrSettingReadOnly
				}
				data, e := base.owner.AppData.Clone()
				if e != nil {
					return e
				}
				data.Stat = &conf.Stat{TreeCount: measured.TreeCount, CTreeCount: measured.CTreeCount, BlockCount: measured.BlockCount, CBlockCount: measured.CBlockCount, DataSize: measured.DataSize, CDataSize: measured.CDataSize, AssetsSize: measured.AssetsSize, CAssetsSize: measured.CAssetsSize}
				if e = base.owner.saveDataLocked(data); e != nil {
					return e
				}
				base.owner.AppData = data
				return nil
			})
			if e != nil {
				return e
			}
			if container == util.ContainerStd {
				warn, e := util.NeedWarnDiskUsageAt(base.workspace, measured.DataSize)
				if e != nil {
					logging.LogWarnf("workspace disk statistics: %s", e)
				} else if warn {
					base.owner.publishSyncUI(c, func() { util.PushMsg(language(179), 7000) })
				}
			}
			return base.guard(c)
		})
	}, nil
}
func StatJobContext(ctx context.Context) error {
	run, e := CaptureStatJob(ctx)
	if e != nil {
		return e
	}
	return run(ctx)
}
