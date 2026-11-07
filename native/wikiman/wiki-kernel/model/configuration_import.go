// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"io"
	"wiki-kernel/conf"
)

func (c *AppConf) ImportConfiguration(ctx context.Context, name string, input io.Reader) error {
	if c == nil {
		return ErrUnboundAppConf
	}
	raw, e := conf.ReadImport(ctx, name, input)
	if e != nil {
		return e
	}
	return c.WithSettingSnapshot(ctx, func(before conf.AppData) error {
		c.m.Lock()
		defer c.m.Unlock()
		if c.persistenceReadOnly {
			return ErrSettingReadOnly
		}
		if c.confPath == "" {
			return ErrUnboundAppConf
		}
		plan, e := before.PrepareImport(ctx, raw)
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = c.saveDataLocked(plan.Data); e != nil {
			return e
		}
		c.AppData = plan.Data
		return nil
	})
}
