package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
)

func TestAppearanceSettingOwnerSaveFailureAndOrderedEffects(t *testing.T) {
	c := settingOwner(t, filepath.Join(t.TempDir(), "conf.json"), false)
	c.Appearance = conf.NewAppearance()
	c.Lang = "en_US"
	input := conf.NewAppearance()
	input.Lang = "zh_CN"
	input.ThemeLight = "missing"
	raw, _ := json.Marshal(input)
	services := conf.SettingServices{NormalizeAppearance: func(ctx context.Context, a *conf.Appearance) (*conf.Appearance, error) {
		return conf.ProjectAppearance(a, nil, nil)
	}}
	var effects []string
	ports := SettingPorts{}
	for _, kind := range []string{"appearanceRuntime", "appearanceMarket", "appearanceNotify"} {
		kind := kind
		ports[kind] = func(ctx context.Context, p conf.SettingPlan, _ conf.SettingEffect) error {
			saved, e := os.ReadFile(c.confPath)
			if e != nil {
				return e
			}
			var d conf.AppData
			if e = json.Unmarshal(saved, &d); e != nil {
				return e
			}
			if d.Appearance.ThemeLight != "daylight" || d.Lang != "zh_CN" {
				t.Fatal("effect preceded normalized save")
			}
			effects = append(effects, kind)
			return nil
		}
	}
	if _, e := c.ApplySettingWithServices(context.Background(), "appearance", raw, ports, services); e != nil || len(effects) != 3 || effects[2] != "appearanceNotify" {
		t.Fatal(e, effects)
	}
	bad := settingOwner(t, filepath.Join(t.TempDir(), "missing", "conf.json"), false)
	bad.Appearance = conf.NewAppearance()
	bad.Lang = "en_US"
	effects = nil
	if _, e := bad.ApplySettingWithServices(context.Background(), "appearance", raw, ports, services); e == nil || len(effects) != 0 || bad.Lang != "en_US" {
		t.Fatal("failed save published appearance", e, effects)
	}
	failure := errors.New("catalog failed")
	services.NormalizeAppearance = func(context.Context, *conf.Appearance) (*conf.Appearance, error) { return nil, failure }
	if _, e := c.ApplySettingWithServices(context.Background(), "appearance", raw, ports, services); !errors.Is(e, failure) {
		t.Fatal(e)
	}
}
