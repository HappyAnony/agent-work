// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"github.com/88250/go-humanize"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func VacuumDataIndexContext(ctx context.Context) error {
	base, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	main, history, assets := sql.CaptureLegacyMainQueue(), sql.CaptureLegacyHistoryQueue(), sql.CaptureLegacyAssetQueue()
	guard := func(c context.Context) error {
		if e := base.guard(c); e != nil {
			return e
		}
		if main.DB != sql.CaptureLegacyMainQueue().DB || history.DB != sql.CaptureLegacyHistoryQueue().DB || assets.DB != sql.CaptureLegacyAssetQueue().DB {
			return conf.ErrBusinessUnbound
		}
		base.owner.m.Lock()
		ro := base.owner.persistenceReadOnly
		base.owner.m.Unlock()
		if ro {
			return ErrSettingReadOnly
		}
		return nil
	}
	var language func(int) string
	if e = base.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		language = func(code int) string { return base.owner.business.Language(v.Lang, code, false, v.CloudRegion) }
		return guard(ctx)
	}); e != nil {
		return e
	}
	return base.run(ctx, func(c, life context.Context) error {
		if e := guard(c); e != nil {
			return e
		}
		base.owner.publishSyncUI(c, func() { util.PushEndlessProgress(language(270)) })
		defer base.owner.publishSyncUI(life, util.PushClearProgress)
		sizes, e := indexdb.Vacuum(c, []*stdsql.DB{main.DB, history.DB, assets.DB}, guard)
		if e != nil {
			return e
		}
		released := int64(0)
		for _, size := range sizes {
			released += size.Before - size.After
		}
		if released < 0 {
			released = 0
		}
		base.owner.publishSyncUI(c, func() { util.PushMsg(fmt.Sprintf(language(271), humanize.BytesCustomCeil(uint64(released), 2)), 7000) })
		return nil
	})
}
