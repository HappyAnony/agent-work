//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/appearance"
)

func TestNativeAppearanceResourcesCandidatesAndWatcherIdentity(t *testing.T) {
	root := fixture(t)
	ctx := context.Background()
	events := make(chan string, 10)
	opts := func(page string) appearance.Options {
		base := filepath.Join(root, ".local", "appearance", page)
		return appearance.Options{Bundled: filepath.Join(base, "bundle"), Destination: filepath.Join(base, "installed"), Themes: filepath.Join(base, "installed", "themes"), Icons: filepath.Join(base, "installed", "icons"), Change: func(c context.Context, e appearance.Event) error {
			if c.Err() != nil {
				return c.Err()
			}
			events <- page
			return nil
		}, Report: func(e error) { t.Error(e) }}
	}
	for _, page := range []string{pageA, pageB} {
		o := opts(page)
		for _, part := range []string{"themes/custom/theme.json", "themes/custom/theme.css", "icons/material/icon.json"} {
			file := filepath.Join(o.Bundled, part)
			os.MkdirAll(filepath.Dir(file), 0700)
			value := `{"version":"` + page + `","modes":["light","dark"]}`
			if filepath.Ext(file) == ".css" {
				value = "body{}"
			}
			if e := os.WriteFile(file, []byte(value), 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: func(_, page string) (conf.AppData, error) {
		a := conf.NewAppearance()
		a.ThemeLight = "custom"
		return conf.AppData{Appearance: a, Lang: a.Lang}, nil
	}, AppearanceOptions: func(_, page string) (appearance.Options, error) { return opts(page), nil }})
	a := getDocLease(t, m, pageA)
	b := getDocLease(t, m, pageB)
	for l, page := range map[*Lease]string{a: pageA, b: pageB} {
		v, e := l.NativeAppearance(ctx)
		if e != nil || v.ThemeVer != page {
			t.Fatal(v, e)
		}
		if _, e = os.Stat(opts(page).Destination); !os.IsNotExist(e) {
			t.Fatal("query installed files", e)
		}
		raw, _ := json.Marshal(conf.NewAppearance())
		plan, e := l.PlanNativeSetting(ctx, "appearance", raw)
		if e != nil || plan.Patches[conf.WorkspacePreference].Set != nil {
			t.Fatal(plan, e)
		}
		if e = l.StartNativeAppearance(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(filepath.Join(opts(pageA).Themes, "custom/theme.css"), []byte("body{color:red}"), 0600); e != nil {
		t.Fatal(e)
	}
	select {
	case page := <-events:
		if page != pageA {
			t.Fatal("foreign watcher", page)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no owner event")
	}
	if e := a.StopNativeAppearance(ctx); e != nil {
		t.Fatal(e)
	}
	a.Release()
	b.Release()
	if e := m.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	if digest(t, root) != before {
		t.Fatal("device resource operations changed source")
	}
}

func TestNativeAppearanceMissingBindingsAndFactoryFailure(t *testing.T) {
	m := open(t, fixture(t), Options{})
	l := getDocLease(t, m, pageA)
	if _, e := l.NativeAppearance(context.Background()); !errors.Is(e, appearance.ErrDependency) {
		t.Fatal(e)
	}
	bad := open(t, fixture(t), Options{AppearanceOptions: func(string, string) (appearance.Options, error) { return appearance.Options{}, nil }})
	if _, e := bad.Acquire(context.Background(), storeID, pageA); !errors.Is(e, appearance.ErrDependency) || bad.Stats().Workspaces != 0 {
		t.Fatal(e)
	}
}
