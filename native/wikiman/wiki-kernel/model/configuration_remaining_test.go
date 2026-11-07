package model

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wiki-kernel/conf"
)

func TestTransientSettingsDoNotPersistOrReplaceEffectsOnlyState(t *testing.T) {
	ctx := context.Background()
	owner := settingOwner(t, filepath.Join(t.TempDir(), "absent", "conf.json"), false)
	if _, e := owner.ApplySetting(ctx, "emoji", []byte(`{"emoji":["faces/a[bad].png","🙂"]}`), nil); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(owner.Editor.Emoji, []string{"faces/abad.png", "🙂"}) {
		t.Fatal(owner.Editor.Emoji)
	}
	editor := owner.Editor
	var effects []string
	ports := SettingPorts{}
	for _, kind := range []string{"virtual", "confNotify"} {
		kind := kind
		ports[kind] = func(context.Context, conf.SettingPlan, conf.SettingEffect) error {
			effects = append(effects, kind)
			return nil
		}
	}
	if _, e := owner.ApplySetting(ctx, "virtualRefresh", []byte(`{}`), ports); e != nil {
		t.Fatal(e)
	}
	if owner.Editor != editor || !reflect.DeepEqual(effects, []string{"virtual", "confNotify"}) {
		t.Fatal("refresh replaced live data", effects)
	}
	effects = nil
	if _, e := owner.ApplySetting(ctx, "virtualInclude", []byte(`{"keywords":[]}`), ports); e != nil || owner.Editor != editor || !reflect.DeepEqual(effects, []string{"confNotify"}) {
		t.Fatal(e, effects)
	}
	if _, e := os.Stat(owner.confPath); !os.IsNotExist(e) {
		t.Fatal("transient action persisted", e)
	}
	failure := errors.New("runtime notification failed")
	ports["confNotify"] = func(context.Context, conf.SettingPlan, conf.SettingEffect) error { return failure }
	if _, e := owner.ApplySetting(ctx, "virtualRefresh", []byte(`{}`), ports); !errors.Is(e, ErrSettingRuntimeEffects) || errors.Is(e, ErrSettingEffects) {
		t.Fatal("false persistent-commit marker", e)
	}
}

func TestExportProbeAuthorityFailureAndPostSaveEffects(t *testing.T) {
	ctx := context.Background()
	owner := settingOwner(t, filepath.Join(t.TempDir(), "conf.json"), false)
	probeCalls := 0
	services := conf.SettingServices{CurrentPandoc: func(context.Context) (string, error) { return "owner-fallback", nil }, ValidatePandoc: func(context.Context, string) (bool, error) { probeCalls++; return false, nil }}
	ports := SettingPorts{"pandocWarning": func(_ context.Context, p conf.SettingPlan, e conf.SettingEffect) error {
		raw, err := os.ReadFile(owner.confPath)
		if err != nil {
			return err
		}
		if len(raw) == 0 || p.Data.Export.PandocBin != "owner-fallback" || e.Text != "bad" {
			t.Fatal("warning preceded save", p, e)
		}
		return nil
	}}
	if _, e := owner.ApplySettingWithServices(ctx, "export", []byte(`{"pandocBin":"bad"}`), ports, services); e != nil || owner.Export.PandocBin != "owner-fallback" {
		t.Fatal(e)
	}
	ro := settingOwner(t, filepath.Join(t.TempDir(), "ro.json"), true)
	if _, e := ro.ApplySettingWithServices(ctx, "export", []byte(`{"pandocBin":"bad"}`), ports, services); !errors.Is(e, ErrSettingReadOnly) || probeCalls != 1 {
		t.Fatal("unauthorized tool probe", e, probeCalls)
	}
}
