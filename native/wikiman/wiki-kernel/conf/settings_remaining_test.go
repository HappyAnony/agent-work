package conf

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestVirtualAndEmojiSettingsRetainOriginalPublicationModes(t *testing.T) {
	before := settingFixture()
	before.Editor.VirtualBlockRefInclude = `First,comma\,key`
	p, e := before.PrepareSetting("virtualInclude", []byte(`{"keywords":[" First ","next,word","next,word"]}`))
	if e != nil {
		t.Fatal(e)
	}
	if p.Data.Editor.VirtualBlockRefInclude != `First,comma\,key,next\,word` || p.NoSave || len(p.Value) != 0 || p.Effects[0].Kind != "virtual" || p.Effects[1].Kind != "confNotify" {
		t.Fatal(p)
	}
	patches, e := p.PreferencePatches(before)
	if e != nil || patches[WorkspacePreference].Set["editor.virtualBlockRefInclude"] == nil {
		t.Fatal(patches, e)
	}
	refresh, e := before.PrepareSetting("virtualRefresh", []byte(`{}`))
	if e != nil || !refresh.NoSave || !refresh.EffectsOnly || len(refresh.Effects) != 2 {
		t.Fatal(refresh, e)
	}
	empty, e := before.PrepareSetting("virtualExclude", []byte(`{"keywords":[]}`))
	if e != nil || !empty.NoSave || !empty.EffectsOnly || len(empty.Effects) != 1 || empty.Effects[0].Kind != "confNotify" {
		t.Fatal(empty, e)
	}
	emoji, e := before.PrepareSetting("emoji", []byte(`{"emoji":["faces/a[bad].png","🙂","api/icon/dynamic<.svg"]}`))
	if e != nil || !emoji.NoSave || emoji.EffectsOnly || len(emoji.Effects) != 0 || !reflect.DeepEqual(emoji.Data.Editor.Emoji, []string{"faces/abad.png", "🙂", "api/icon/dynamic<.svg"}) {
		t.Fatal(emoji, e)
	}
	patches, e = emoji.PreferencePatches(before)
	if e != nil || len(patches) != 0 {
		t.Fatal("transient emoji became persistent", patches, e)
	}
	for _, q := range []struct{ kind, raw string }{{"emoji", `{"emoji":[null]}`}, {"emoji", `{}`}, {"virtualInclude", `{"keywords":[1]}`}, {"virtualExclude", `{"keywords":null}`}} {
		if _, e = before.PrepareSetting(q.kind, []byte(q.raw)); e == nil {
			t.Fatal(q)
		}
	}
}

func TestExportSettingsUseExplicitProbeAndOriginalFallback(t *testing.T) {
	before := settingFixture()
	ctx := context.Background()
	calls := 0
	services := SettingServices{CurrentPandoc: func(context.Context) (string, error) { return "owner-tool", nil }, ValidatePandoc: func(c context.Context, p string) (bool, error) { calls++; return p == "valid", c.Err() }}
	valid, e := before.PrepareSettingWithServices(ctx, "export", []byte(`{"pandocBin":"valid","docxTemplate":"device-template"}`), services)
	if e != nil || valid.Data.Export.PandocBin != "valid" || valid.Effects[0].Kind != "pandoc" {
		t.Fatal(valid, e)
	}
	invalid, e := before.PrepareSettingWithServices(ctx, "export", []byte(`{"pandocBin":"invalid"}`), services)
	if e != nil || invalid.Data.Export.PandocBin != "owner-tool" || invalid.Effects[0].Kind != "pandocWarning" || invalid.Effects[0].Text != "invalid" {
		t.Fatal(invalid, e)
	}
	patches, e := valid.PreferencePatches(before)
	if e != nil {
		t.Fatal(e)
	}
	for _, patch := range patches {
		if patch.Set["export.pandocBin"] != nil {
			t.Fatal("runtime capability became source preference")
		}
	}
	if _, e = before.PrepareSetting("export", []byte(`{"pandocBin":"valid"}`)); !errors.Is(e, ErrBusinessUnbound) {
		t.Fatal(e)
	}
	empty, e := before.PrepareSetting("export", []byte(`{}`))
	if e != nil || len(empty.Effects) != 0 || calls != 2 {
		t.Fatal(empty, e, calls)
	}
	failed := errors.New("probe failed")
	services.ValidatePandoc = func(context.Context, string) (bool, error) { return false, failed }
	if _, e = before.PrepareSettingWithServices(ctx, "export", []byte(`{"pandocBin":"invalid"}`), services); !errors.Is(e, failed) {
		t.Fatal(e)
	}
}
