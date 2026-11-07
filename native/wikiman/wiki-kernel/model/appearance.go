// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"context"
	"github.com/siyuan-note/logging"
	"path/filepath"
	"sync"
	"wiki-kernel/conf"
	"wiki-kernel/internal/appearance"
	"wiki-kernel/util"
)

var legacyAppearance struct {
	sync.Mutex
	owner     *AppConf
	resources *appearance.Owner
}

func legacyAppearanceOwner(owner *AppConf) (*appearance.Owner, error) {
	if owner == nil || owner != Conf {
		return nil, conf.ErrBusinessUnbound
	}
	if e := RegisterLegacyServiceCloser("appearance", closeLegacyAppearance); e != nil {
		return nil, e
	}
	legacyAppearance.Lock()
	defer legacyAppearance.Unlock()
	if legacyAppearance.resources != nil && legacyAppearance.owner != owner {
		if e := legacyAppearance.resources.Close(context.Background()); e != nil {
			return nil, e
		}
		legacyAppearance.resources = nil
	}
	if legacyAppearance.resources == nil {
		value, e := appearance.New(appearance.Options{Bundled: filepath.Join(util.WorkingDir, "appearance"), Destination: util.AppearancePath, Themes: util.ThemesPath, Icons: util.IconsPath, AllowLinkedDirectories: true,
			Change: func(ctx context.Context, event appearance.Event) error {
				if e := ctx.Err(); e != nil {
					return e
				}
				if Conf != owner {
					return conf.ErrBusinessUnbound
				}
				util.BroadcastByType("main", "refreshtheme", 0, "", map[string]interface{}{"theme": event.URL})
				return nil
			},
			Report: func(e error) { logging.LogErrorf("appearance watcher failed: %s", e) },
		})
		if e != nil {
			return nil, e
		}
		legacyAppearance.resources = value
		legacyAppearance.owner = owner
	}
	return legacyAppearance.resources, nil
}
func closeLegacyAppearance(ctx context.Context) error {
	legacyAppearance.Lock()
	defer legacyAppearance.Unlock()
	if legacyAppearance.resources == nil {
		return nil
	}
	return legacyAppearance.resources.Close(ctx)
}
func closeThemeWatchers() {
	legacyAppearance.Lock()
	defer legacyAppearance.Unlock()
	if legacyAppearance.resources != nil {
		if e := legacyAppearance.resources.Stop(context.Background()); e != nil {
			logging.LogErrorf("close theme watchers failed: %s", e)
		}
	}
}
func InitAppearance() {
	util.SetBootDetails("Initializing appearance...")
	if e := InitAppearanceContext(context.Background()); e != nil {
		logging.LogErrorf("initialize appearance failed: %s", e)
	}
}
func InitAppearanceContext(ctx context.Context) error {
	owner := Conf
	if owner == nil {
		return conf.ErrBusinessUnbound
	}
	return owner.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		resources, e := legacyAppearanceOwner(owner)
		if e != nil {
			return e
		}
		input := data.Appearance
		if input == nil {
			input = conf.NewAppearance()
		}
		if data.Lang != "" {
			input.Lang = data.Lang
		}
		normalized, e := resources.Prepare(ctx, input)
		if e != nil {
			return e
		}
		data.Appearance = normalized
		data.Lang = normalized.Lang
		owner.m.Lock()
		e = owner.saveDataLocked(data)
		if e == nil {
			owner.AppData = data
		}
		owner.m.Unlock()
		if e != nil {
			return e
		}
		if e = resources.Apply(ctx, normalized); e != nil {
			return e
		}
		util.StatusBarCfg = normalized.StatusBar
		util.Lang = normalized.Lang
		util.InitEmojiChars()
		return nil
	})
}
func ReloadIcon() {
	if e := ReloadIconContext(context.Background()); e != nil {
		logging.LogErrorf("reload icons failed: %s", e)
	}
}

func ReloadIconContext(ctx context.Context) error {
	o, e := captureLegacyReindexOwner(ctx)
	if e != nil {
		return e
	}
	return o.run(ctx, func(op, life context.Context) error {
		return o.owner.WithSettingSnapshot(op, func(data conf.AppData) error {
			resources, e := legacyAppearanceOwner(o.owner)
			if e != nil {
				return e
			}
			normalized, e := resources.Prepare(op, data.Appearance)
			if e != nil {
				return e
			}
			if e = o.guard(op); e != nil {
				return e
			}
			o.owner.m.Lock()
			defer o.owner.m.Unlock()
			if o.owner.Appearance == nil {
				return conf.ErrBusinessUnbound
			}
			o.owner.Appearance.Icons = append([]string(nil), normalized.Icons...)
			if o.owner.Appearance.Icon == normalized.Icon {
				o.owner.Appearance.IconVer = normalized.IconVer
			}
			return nil
		})
	})
}
