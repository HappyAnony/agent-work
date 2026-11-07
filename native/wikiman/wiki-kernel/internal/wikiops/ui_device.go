// SPDX-License-Identifier: AGPL-3.0-or-later
package wikiops

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	"wiki-kernel/conf"
	"wiki-kernel/internal/workspace"
)

var uiDeviceSettings = map[string]string{
	"/api/setting/setAppearance":    "appearance",
	"/api/system/setAppearanceMode": "appearanceMode",
	"/api/setting/setKeymap":        "keymap",
}

var uiKeyName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

func validateUIKeymap(raw []byte) bool {
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil || keys == nil {
		return false
	}
	var validate func(map[string]json.RawMessage, int) bool
	validate = func(group map[string]json.RawMessage, depth int) bool {
		if depth > 4 || len(group) > 1000 {
			return false
		}
		for name, value := range group {
			if name == "custom" || name == "default" {
				var hotkey string
				if json.Unmarshal(value, &hotkey) != nil || len(hotkey) > 128 || strings.ContainsAny(hotkey, "\x00\r\n") {
					return false
				}
				continue
			}
			var children map[string]json.RawMessage
			if !uiKeyName.MatchString(name) || json.Unmarshal(value, &children) != nil || children == nil || !validate(children, depth+1) {
				return false
			}
		}
		return true
	}
	return validate(keys, 0)
}

func projectUIAppearance(value *conf.Appearance, themes []conf.AppearanceThemeRecord) (*conf.Appearance, error) {
	return conf.ProjectAppearance(value, themes, []conf.AppearanceIconRecord{{Name: "ant"}, {Name: "material"}})
}

func saveUIDeviceSetting(ctx context.Context, m *workspace.WorkspaceManager, r UIRequest, kind string, themes []conf.AppearanceThemeRecord) (any, error) {
	var doc conf.PreferenceDocument
	var hash string
	var response any
	var changed bool
	err := m.WithReadContext(ctx, r.StoreID, func(c context.Context) error {
		tree, err := m.Tree(c, r.StoreID)
		if err != nil {
			return err
		}
		if page, ok := tree.Get(r.PageID); !ok || page.DeletedAt != nil {
			return fs.ErrNotExist
		}
		l, err := m.Acquire(c, r.StoreID, r.PageID)
		if err != nil {
			return err
		}
		defer l.Release()
		cfg, err := l.Configuration(c)
		if err != nil {
			return err
		}
		if cfg.ReadOnly || cfg.Editor != nil && cfg.Editor.ReadOnly {
			return ErrUnauthorized
		}
		if cfg.Appearance == nil {
			cfg.Appearance = conf.NewAppearance()
			cfg.Appearance.Lang = cfg.Lang
		}
		data := r.Data
		if kind == "keymap" {
			var input struct{ Data json.RawMessage }
			if Decode(bytes.NewReader(data), &input) != nil || !validateUIKeymap(input.Data) {
				return ErrArgument
			}
			data = input.Data
		}
		plan, err := cfg.PrepareSettingWithServices(c, kind, data, conf.SettingServices{
			NormalizeAppearance: func(_ context.Context, value *conf.Appearance) (*conf.Appearance, error) {
				if value.Mode < 0 || value.Mode > 1 || !slices.ContainsFunc(conf.PortableLanguages(), func(lang *conf.Lang) bool {
					return lang.Name == value.Lang
				}) {
					return nil, ErrArgument
				}
				return projectUIAppearance(value, themes)
			},
			ThemeScriptExists: func(context.Context, string) (bool, error) { return false, nil },
		})
		if err != nil {
			return err
		}
		if kind == "appearanceMode" {
			if plan.Data.Appearance.Mode < 0 || plan.Data.Appearance.Mode > 1 {
				return ErrArgument
			}
			appearance, e := projectUIAppearance(plan.Data.Appearance, themes)
			if e != nil {
				return e
			}
			response = map[string]any{"appearance": appearance}
		}
		patches, err := plan.PreferencePatches(cfg)
		if err != nil {
			return err
		}
		patch, ok := patches[conf.DevicePreference]
		if !ok {
			return nil
		}
		if len(patches) != 1 {
			return ErrUnauthorized
		}
		allowed := func(field string) bool {
			if kind == "keymap" {
				return field == "keymap"
			}
			return field == "lang" || strings.HasPrefix(field, "appearance.")
		}
		for field := range patch.Set {
			if !allowed(field) {
				return ErrUnauthorized
			}
		}
		for _, field := range patch.Remove {
			if !allowed(field) {
				return ErrUnauthorized
			}
		}
		doc, hash, err = m.DevicePreferences(c, r.StoreID)
		if err != nil {
			return err
		}
		maps.Copy(doc.Values, patch.Set)
		for _, field := range patch.Remove {
			delete(doc.Values, field)
		}
		changed = true
		return nil
	})
	if err == nil && changed {
		_, err = m.SaveDevicePreferences(ctx, r.StoreID, hash, doc)
	}
	return response, err
}
