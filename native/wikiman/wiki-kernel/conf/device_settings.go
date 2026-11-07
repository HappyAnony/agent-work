// SPDX-License-Identifier: AGPL-3.0-or-later
package conf

import (
	"context"
	"encoding/json"
	"github.com/88250/gulu"
	"strings"
)

// Device fields remain private preferences. Planning does not configure the
// process or imply permission to change any workspace source.
func prepareDeviceSetting(ctx context.Context, out SettingPlan, kind string, raw json.RawMessage, services SettingServices) (SettingPlan, error) {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(raw, &args); err != nil || args == nil {
		return SettingPlan{}, ErrSetting
	}
	decode := func(key string, dst any) error {
		v, ok := args[key]
		if !ok || string(v) == "null" {
			return ErrSetting
		}
		return json.Unmarshal(v, dst)
	}
	var target any
	var err error
	switch kind {
	case "uiLayout":
		v := &UILayout{}
		err = decode("layout", v)
		out.Data.UILayout = v
		target = v
	case "apiToken":
		if out.Data.Api == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		err = decode("token", &out.Data.Api.Token)
		target = out.Data.Api.Token
	case "accessAuthCode":
		var v string
		err = decode("accessAuthCode", &v)
		if v != "*******" {
			out.Data.AccessAuthCode = gulu.Str.RemovePUA(gulu.Str.RemoveInvisible(strings.TrimSpace(v)))
		}
		target = out.Data.AccessAuthCode
	case "appearanceMode":
		if out.Data.Appearance == nil || services.ThemeScriptExists == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		err = decode("mode", &out.Data.Appearance.Mode)
		if err == nil {
			theme := out.Data.Appearance.ThemeDark
			if out.Data.Appearance.Mode == 0 {
				theme = out.Data.Appearance.ThemeLight
			}
			out.Data.Appearance.ThemeJS, err = services.ThemeScriptExists(ctx, theme)
		}
		target = out.Data.Appearance
	default:
		if out.Data.System == nil {
			return SettingPlan{}, ErrBusinessUnbound
		}
		switch kind {
		case "lockScreenMode":
			err = decode("lockScreenMode", &out.Data.System.LockScreenMode)
		case "networkServe":
			err = decode("networkServe", &out.Data.System.NetworkServe)
			out.Effects = []SettingEffect{{Kind: "deviceNetworkServe"}}
		case "autoLaunch":
			err = decode("autoLaunch", &out.Data.System.AutoLaunch2)
		case "downloadInstallPkg":
			err = decode("downloadInstallPkg", &out.Data.System.DownloadInstallPkg)
		case "networkProxy":
			v := &NetworkProxy{}
			if err = decode("scheme", &v.Scheme); err == nil {
				err = decode("host", &v.Host)
			}
			if err == nil {
				err = decode("port", &v.Port)
			}
			out.Data.System.NetworkProxy = v
			out.Effects = []SettingEffect{{Kind: "deviceNetworkProxy"}}
		case "defenderExcluded":
			out.Data.System.MicrosoftDefenderExcluded = true
		default:
			return SettingPlan{}, ErrSetting
		}
		target = out.Data.System
	}
	if err != nil {
		return SettingPlan{}, err
	}
	if err = ctx.Err(); err != nil {
		return SettingPlan{}, err
	}
	out.Value, err = json.Marshal(target)
	return out, err
}
