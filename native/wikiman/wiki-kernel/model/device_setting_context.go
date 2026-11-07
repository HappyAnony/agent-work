// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func SetDeviceConfigurationContext(ctx context.Context, kind string, raw json.RawMessage) (out json.RawMessage, err error) {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	ui, err := captureLegacyDelayedUIOwner(ctx)
	if err != nil {
		return nil, err
	}
	themes := util.ThemesPath
	raw = append(json.RawMessage(nil), raw...)
	err = base.run(ctx, func(c, life context.Context) error {
		ports := SettingPorts{}
		ports["deviceNetworkProxy"] = func(_ context.Context, p conf.SettingPlan, _ conf.SettingEffect) error {
			if !ui.publish(life, func() {
				util.SetNetworkProxy(p.Data.System.NetworkProxy.String())
				util.PushMsg(ui.language(102, true), 3000)
			}) {
				return conf.ErrBusinessUnbound
			}
			return nil
		}
		ports["deviceNetworkServe"] = func(_ context.Context, _ conf.SettingPlan, _ conf.SettingEffect) error {
			if !ui.publish(life, func() { util.PushMsg(ui.language(42, true), 15000) }) {
				return conf.ErrBusinessUnbound
			}
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-life.Done():
				return life.Err()
			}
		}
		var e error
		out, e = base.owner.ApplySettingWithServices(c, kind, raw, ports, conf.SettingServices{ThemeScriptExists: func(ctx context.Context, theme string) (bool, error) {
			if e := base.guard(ctx); e != nil {
				return false, e
			}
			if themes != util.ThemesPath || theme == "" || filepath.Base(theme) != theme {
				return false, conf.ErrBusinessUnbound
			}
			root, e := os.OpenRoot(themes)
			if e != nil {
				return false, e
			}
			_, e = root.Stat(filepath.Join(theme, "theme.js"))
			closeErr := root.Close()
			if errors.Is(e, fs.ErrNotExist) {
				e = nil
				return false, errors.Join(e, closeErr, base.guard(ctx))
			}
			return e == nil, errors.Join(e, closeErr, base.guard(ctx))
		}})
		return e
	})
	if err != nil {
		out = nil
	}
	return
}

func ConfigurationPresentationContext(ctx context.Context, restricted, publish bool) (out *AppConf, err error) {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return nil, err
	}
	err = base.run(ctx, func(c, _ context.Context) error {
		var e error
		out, e = base.owner.Presentation(restricted, publish)
		return e
	})
	if err != nil {
		out = nil
	}
	return
}
