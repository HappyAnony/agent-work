//go:build fts5

package workspace

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/conf"
)

func TestContextPathsAndFullConfigurationPresentation(t *testing.T) {
	root := fixture(t)
	root, _ = filepath.EvalSymlinks(root)
	before := digest(t, root)
	ctx := context.Background()
	m := open(t, root, Options{Configuration: func(_, page string) (conf.AppData, error) {
		return conf.AppData{Lang: page, AccessAuthCode: "private", UserData: "private", System: &conf.System{DataDir: "/untrusted", ConfDir: "/untrusted", WorkspaceDir: "/untrusted"}, Sync: conf.NewSync()}, nil
	}, ConfigurationBusiness: func(_, page string) (*conf.Business, error) {
		return conf.NewBusiness(nil, map[string]map[int]string{page: {53: page}}, nil), nil
	}})
	a := getDocLease(t, m, pageA)
	b := getDocLease(t, m, pageB)
	for l, page := range map[*Lease]string{a: pageA, b: pageB} {
		p, e := l.RuntimePaths(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if p.DataDir != filepath.Join(root, "pages", page, "data") {
			t.Fatal(p)
		}
		d, e := l.Configuration(ctx)
		if e != nil || d.System.DataDir != p.DataDir || d.System.ConfDir != p.ConfDir || d.System.WorkspaceDir != filepath.Dir(p.DataDir) {
			t.Fatal(d.System, e)
		}
		shown, e := l.ConfigurationPresentation(ctx, false, true)
		if e != nil || shown.Sync.Stat != page || !shown.ReadOnly || shown.UserData != "" || shown.AccessAuthCode != "*******" {
			t.Fatal(shown, e)
		}
		restricted, e := l.ConfigurationPresentation(ctx, true, false)
		if e != nil || restricted.System.DataDir != "" {
			t.Fatal(e)
		}
		name, e := l.RuntimePath(ctx, "assets", "image.png")
		if e != nil || name != filepath.Join(p.DataDir, "assets", "image.png") {
			t.Fatal(name, e)
		}
		for _, bad := range []string{"../escape", "/absolute", "a\\b"} {
			if _, e = l.RuntimePath(ctx, "data", bad); e == nil {
				t.Fatal(bad)
			}
		}
		if _, e = l.RuntimePath(ctx, "unknown", ""); e == nil {
			t.Fatal("unknown root accepted")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := a.RuntimePaths(canceled); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	a.Release()
	if _, e := a.ConfigurationPresentation(ctx, false, false); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
	if digest(t, root) != before {
		t.Fatal("read mutated source")
	}
}

func TestOwnedEmojisUsedByActualParserAndRefresh(t *testing.T) {
	root := fixture(t)
	ctx := context.Background()
	for page, ext := range map[string]string{pageA: "png", pageB: "svg"} {
		dir := filepath.Join(root, "pages", page, "data", "emojis")
		if e := os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(filepath.Join(dir, "r02-owner."+ext), []byte("synthetic"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	before := digest(t, root)
	m := open(t, root, Options{Configuration: func(string, string) (conf.AppData, error) {
		return conf.AppData{Editor: conf.NewEditor(), Export: conf.NewExport()}, nil
	}})
	a := getDocLease(t, m, pageA)
	b := getDocLease(t, m, pageB)
	for l, ext := range map[*Lease]string{a: "png", b: "svg"} {
		if e := l.WithConfiguredParser(ctx, nil, func(engine *lute.Lute) error {
			if engine.GetEmojis()["r02-owner"] != "/emojis/r02-owner."+ext {
				t.Fatal(engine.GetEmojis()["r02-owner"])
			}
			engine.PutEmojis(map[string]string{"borrowed": "outside"})
			return nil
		}); e != nil {
			t.Fatal(e)
		}
		got, e := l.CustomEmojis(ctx)
		if e != nil || got["borrowed"] != "" {
			t.Fatal(got, e)
		}
		got["r02-owner"] = "outside"
	}
	if digest(t, root) != before {
		t.Fatal("emoji read repaired source")
	}
	if e := os.Remove(filepath.Join(root, "pages", pageA, "data", "emojis", "r02-owner.png")); e != nil {
		t.Fatal(e)
	}
	if e := a.WithConfiguredParser(ctx, nil, func(engine *lute.Lute) error {
		if engine.GetEmojis()["r02-owner"] != "" {
			t.Fatal("deleted emoji remained")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(t.TempDir(), filepath.Join(root, "pages", pageA, "data", "emojis", "escape")); e != nil {
		t.Fatal(e)
	}
	if _, e := a.CustomEmojis(ctx); e == nil {
		t.Fatal("emoji symlink accepted")
	}
	if e := b.WithConfiguredParser(ctx, nil, func(engine *lute.Lute) error {
		if engine.GetEmojis()["r02-owner"] != "/emojis/r02-owner.svg" {
			t.Fatal("A invalidated B")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	a.Release()
	if _, e := a.CustomEmojis(ctx); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
}
