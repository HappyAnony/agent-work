// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package conf

import "sort"

type AppearanceThemeRecord struct {
	Name, Version, ChineseName string
	Modes                      []string
	JavaScript                 bool
}
type AppearanceIconRecord struct{ Name, Version string }

// ProjectAppearance retains original lists, labels, ordering and fallback names.
// Metadata is resolved after fallback, so versions match the actual selection.
func ProjectAppearance(input *Appearance, themes []AppearanceThemeRecord, icons []AppearanceIconRecord) (*Appearance, error) {
	if input == nil {
		return nil, ErrBusinessUnbound
	}
	copy, e := (AppData{Appearance: input}).Clone()
	if e != nil {
		return nil, e
	}
	out := copy.Appearance
	themes = append([]AppearanceThemeRecord(nil), themes...)
	icons = append([]AppearanceIconRecord(nil), icons...)
	sort.SliceStable(themes, func(i, j int) bool { return themes[i].Name < themes[j].Name })
	sort.SliceStable(icons, func(i, j int) bool { return icons[i].Name < icons[j].Name })
	label := func(name, cn string) string {
		if out.Lang == "zh_CN" {
			if name == "midnight" || name == "daylight" {
				return name + "（默认主题）"
			}
			if cn != "" && cn != name {
				return cn + "（" + name + "）"
			}
		} else if name == "midnight" || name == "daylight" {
			return name + " (Default)"
		}
		return name
	}
	out.DarkThemes = []*AppearanceTheme{{Name: "midnight", Label: label("midnight", "")}}
	out.LightThemes = []*AppearanceTheme{{Name: "daylight", Label: label("daylight", "")}}
	dark, light := map[string]bool{"midnight": true}, map[string]bool{"daylight": true}
	metadata := map[string]AppearanceThemeRecord{}
	for _, theme := range themes {
		metadata[theme.Name] = theme
		for _, mode := range theme.Modes {
			if theme.Name == "midnight" || theme.Name == "daylight" {
				continue
			}
			t := &AppearanceTheme{Name: theme.Name, Label: label(theme.Name, theme.ChineseName)}
			if mode == "dark" {
				out.DarkThemes = append(out.DarkThemes, t)
				dark[t.Name] = true
			} else if mode == "light" {
				out.LightThemes = append(out.LightThemes, t)
				light[t.Name] = true
			}
		}
	}
	if !dark[out.ThemeDark] {
		out.ThemeDark = "midnight"
	}
	if !light[out.ThemeLight] {
		out.ThemeLight = "daylight"
	}
	selected := out.ThemeLight
	if out.Mode != 0 {
		selected = out.ThemeDark
	}
	theme := metadata[selected]
	out.ThemeVer = theme.Version
	out.ThemeJS = theme.JavaScript
	out.Icons = nil
	iconMeta := map[string]string{}
	for _, icon := range icons {
		out.Icons = append(out.Icons, icon.Name)
		iconMeta[icon.Name] = icon.Version
	}
	if _, ok := iconMeta[out.Icon]; !ok {
		out.Icon = "material"
	}
	out.IconVer = iconMeta[out.Icon]
	return out, nil
}
