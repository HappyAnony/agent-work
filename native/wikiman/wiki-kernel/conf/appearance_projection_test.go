package conf

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestOriginalAppearanceProjectionLabelsModesFallbackAndOwnership(t *testing.T) {
	input := NewAppearance()
	input.Lang = "zh_CN"
	input.ThemeLight = "custom"
	input.ThemeDark = "missing"
	input.Icon = "missing"
	themes := []AppearanceThemeRecord{{Name: "custom", Version: "2", ChineseName: "自定义", Modes: []string{"light", "dark"}, JavaScript: true}, {Name: "daylight", Version: "1", Modes: []string{"light"}}, {Name: "midnight", Version: "1d", Modes: []string{"dark"}}}
	icons := []AppearanceIconRecord{{Name: "material", Version: "3"}, {Name: "ant", Version: "4"}}
	out, e := ProjectAppearance(input, themes, icons)
	if e != nil {
		t.Fatal(e)
	}
	if out.LightThemes[0].Label != "daylight（默认主题）" || out.LightThemes[1].Label != "自定义（custom）" || out.ThemeDark != "midnight" || out.ThemeVer != "2" || !out.ThemeJS || out.Icon != "material" || out.IconVer != "3" || out.Icons[0] != "ant" {
		t.Fatal(out)
	}
	out.StatusBar.MsgTaskHistoryGenerateFileDisabled = true
	if input.StatusBar.MsgTaskHistoryGenerateFileDisabled {
		t.Fatal("borrowed status bar")
	}
	input.ThemeLight = "missing"
	input.Lang = "en_US"
	out, e = ProjectAppearance(input, themes, icons)
	if e != nil || out.ThemeLight != "daylight" || out.ThemeVer != "1" || out.ThemeJS || out.LightThemes[0].Label != "daylight (Default)" {
		t.Fatal(out, e)
	}
	empty, e := ProjectAppearance(NewAppearance(), nil, nil)
	if e != nil || empty.DarkThemes[0] == nil || empty.LightThemes[0] == nil {
		t.Fatal("missing builtins created nil entries", e)
	}
}

func TestAppearanceSettingUsesExplicitCatalogAndClassifiesDeviceValues(t *testing.T) {
	data := AppData{Appearance: NewAppearance(), Lang: "en_US"}
	raw, _ := json.Marshal(NewAppearance())
	if _, e := data.PrepareSetting("appearance", raw); !errors.Is(e, ErrBusinessUnbound) {
		t.Fatal(e)
	}
	input := NewAppearance()
	input.Lang = "zh_CN"
	raw, _ = json.Marshal(input)
	services := SettingServices{NormalizeAppearance: func(ctx context.Context, a *Appearance) (*Appearance, error) { return ProjectAppearance(a, nil, nil) }}
	plan, e := data.PrepareSettingWithServices(context.Background(), "appearance", raw, services)
	if e != nil || plan.Data.Lang != "zh_CN" || len(plan.Effects) != 3 || plan.Effects[0].Kind != "appearanceRuntime" || plan.Effects[2].Kind != "appearanceNotify" {
		t.Fatal(plan, e)
	}
	patches, e := plan.PreferencePatches(data)
	if e != nil || patches[DevicePreference].Set["lang"] == nil || len(patches[WorkspacePreference].Set) > 0 || len(patches[StorePreference].Set) > 0 {
		t.Fatal(patches, e)
	}
	if _, e := data.PrepareSettingWithServices(context.Background(), "appearance", []byte(`{"statusBar":null}`), services); e == nil {
		t.Fatal("nil status bar accepted")
	}
}
