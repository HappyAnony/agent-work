//go:build fts5

package workspace

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/conf"
)

func TestNativeRemainingSettingsBindActualDeviceServicesAndPageState(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	ctx := context.Background()
	m := open(t, root, Options{Configuration: func(_, page string) (conf.AppData, error) {
		d := conf.AppData{Editor: conf.NewEditor(), Export: conf.NewExport()}
		d.Editor.VirtualBlockRefInclude = page
		return d, nil
	}, SettingServices: func(_, page string) (conf.SettingServices, error) {
		return conf.SettingServices{CurrentPandoc: func(context.Context) (string, error) { return page, nil }, ValidatePandoc: func(c context.Context, p string) (bool, error) { return false, c.Err() }}, nil
	}})
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		export, e := l.PlanNativeSetting(ctx, "export", []byte(`{"pandocBin":"bad","docxTemplate":"device-template"}`))
		if e != nil || export.Setting.Data.Export.PandocBin != page {
			t.Fatal(export, e)
		}
		for _, patch := range export.Patches {
			if patch.Set["export.pandocBin"] != nil {
				t.Fatal("runtime tool path in preference patch")
			}
		}
		keywords, e := l.PlanNativeSetting(ctx, "virtualInclude", []byte(`{"keywords":["next,word"]}`))
		if e != nil || keywords.Setting.Data.Editor.VirtualBlockRefInclude != page+`,next\,word` {
			t.Fatal(keywords, e)
		}
		emoji, e := l.PlanNativeSetting(ctx, "emoji", []byte(`{"emoji":["faces/a[bad].png"]}`))
		if e != nil || len(emoji.Patches) != 0 || !emoji.Setting.NoSave {
			t.Fatal(emoji, e)
		}
		unchanged, e := l.Configuration(ctx)
		if e != nil || unchanged.Editor.VirtualBlockRefInclude != page || len(unchanged.Editor.Emoji) != 0 {
			t.Fatal("preview published", e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("setting preview changed source")
	}
	unbound := open(t, fixture(t), Options{})
	l := getDocLease(t, unbound, pageA)
	if _, e := l.PlanNativeSetting(ctx, "export", []byte(`{"pandocBin":"foreign"}`)); !errors.Is(e, conf.ErrBusinessUnbound) {
		t.Fatal(e)
	}
}

func TestSettingServiceAcquireFailurePublishesNoContext(t *testing.T) {
	failure := errors.New("setting provider failed")
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{SettingServices: func(string, string) (conf.SettingServices, error) { return conf.SettingServices{}, failure }})
	if _, e := m.Acquire(context.Background(), storeID, pageA); !errors.Is(e, failure) || m.Stats().Workspaces != 0 || digest(t, root) != before {
		t.Fatal(e)
	}
}
