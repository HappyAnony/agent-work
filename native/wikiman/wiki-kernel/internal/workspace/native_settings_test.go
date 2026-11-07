//go:build fts5

package workspace

import (
	"context"
	"errors"
	"testing"
	"wiki-kernel/conf"
)

func TestNativeSettingActualOwnersClassifyWithoutPublication(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	ctx := context.Background()
	m := open(t, root, Options{Configuration: func(_, page string) (conf.AppData, error) {
		d := conf.AppData{Editor: conf.NewEditor(), FileTree: conf.NewFileTree(), Search: conf.NewSearch()}
		d.Editor.Markdown.InlineMath = page == pageA
		return d, nil
	}})
	a := getDocLease(t, m, pageA)
	b := getDocLease(t, m, pageB)
	for l, want := range map[*Lease]bool{a: true, b: false} {
		p, e := l.PlanNativeSetting(ctx, "editor", []byte(`{"fontSize":31,"readOnly":true,"plantUMLServePath":"explicit-device-path"}`))
		if e != nil {
			t.Fatal(e)
		}
		if p.Setting.Data.Editor.Markdown.InlineMath != want || string(p.Patches[conf.DevicePreference].Set["editor.fontSize"]) != "31" {
			t.Fatal("wrong owner/default", p)
		}
		for _, patch := range p.Patches {
			if _, ok := patch.Set["editor.readOnly"]; ok {
				t.Fatal("runtime authority persisted")
			}
		}
		p.Setting.Data.Editor.FontSize = 99
		d, e := l.Configuration(ctx)
		if e != nil || d.Editor.FontSize == 31 || d.Editor.FontSize == 99 || d.Editor.ReadOnly {
			t.Fatal("candidate published memory", e)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := a.PlanNativeSetting(canceled, "ai", []byte(`{}`)); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := a.PlanNativeSetting(ctx, "ai", []byte(`{}`)); e == nil {
		t.Fatal("invalid AI accepted")
	}
	a.Release()
	if _, e := a.PlanNativeSetting(ctx, "search", []byte(`{}`)); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
	if digest(t, root) != before {
		t.Fatal("candidate changed source")
	}
}
