package model

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"wiki-kernel/bazaar"
	"wiki-kernel/util"
)

type bazaarMutationRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *bazaarMutationRecorder) add(kind, target, systemID string) {
	r.mu.Lock()
	r.calls = append(r.calls, kind+":"+target+":"+systemID)
	r.mu.Unlock()
}

func mutationPorts(r *bazaarMutationRecorder) legacyBazaarMutationPorts {
	install := func(kind string) func(context.Context, string, string, string, string, ...bazaar.BackgroundScheduler) error {
		return func(ctx context.Context, _, _, target, systemID string, schedulers ...bazaar.BackgroundScheduler) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(schedulers) != 1 || schedulers[0] == nil {
				return errors.New("download count owner missing")
			}
			r.add("install-"+kind, target, systemID)
			return nil
		}
	}
	uninstall := func(kind string) func(context.Context, string) error {
		return func(ctx context.Context, target string) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			r.add("uninstall-"+kind, target, "")
			return nil
		}
	}
	return legacyBazaarMutationPorts{
		installPlugin: install("plugin"), installWidget: install("widget"), installIcon: install("icon"),
		installTheme: install("theme"), installTemplate: install("template"),
		uninstallPlugin: uninstall("plugin"), uninstallWidget: uninstall("widget"), uninstallIcon: uninstall("icon"),
		uninstallTheme: uninstall("theme"), uninstallTemplate: uninstall("template"),
		initAppearance: func(ctx context.Context) error { return ctx.Err() }, closeThemeWatchers: func() {},
	}
}

func TestBazaarMutationOwnerCoversFiveInstallAndUninstallFamilies(t *testing.T) {
	legacyBackgroundOwnerFixture(t)
	recorder := &bazaarMutationRecorder{}
	ports := mutationPorts(recorder)
	ctx := context.Background()
	if _, err := installBazaarPluginContext(ctx, "repo", "hash", "plug", "", "", false, ports); err != nil {
		t.Fatal(err)
	}
	if _, err := uninstallBazaarPluginContext(ctx, "plug", "", "", false, ports); err != nil {
		t.Fatal(err)
	}
	if _, err := installBazaarWidgetContext(ctx, "repo", "hash", "wid", "", false, ports); err != nil {
		t.Fatal(err)
	}
	if _, err := uninstallBazaarWidgetContext(ctx, "wid", "", false, ports); err != nil {
		t.Fatal(err)
	}
	if _, err := installBazaarIconContext(ctx, "repo", "hash", "ico", "", false, ports); err != nil {
		t.Fatal(err)
	}
	if _, err := uninstallBazaarIconContext(ctx, "ico", "", false, ports); err != nil {
		t.Fatal(err)
	}
	if _, err := installBazaarTemplateContext(ctx, "repo", "hash", "tmpl", "", false, ports); err != nil {
		t.Fatal(err)
	}
	if _, err := uninstallBazaarTemplateContext(ctx, "tmpl", "", false, ports); err != nil {
		t.Fatal(err)
	}
	if _, err := installBazaarThemeContext(ctx, "repo", "hash", "thm", 1, false, "", false, ports); err != nil {
		t.Fatal(err)
	}
	if _, err := uninstallBazaarThemeContext(ctx, "thm", "", false, ports); err != nil {
		t.Fatal(err)
	}
	if len(recorder.calls) != 10 {
		t.Fatal(recorder.calls)
	}
	wantTargets := []string{
		filepath.Join(util.DataDir, "plugins", "plug"), filepath.Join(util.DataDir, "plugins", "plug"),
		filepath.Join(util.DataDir, "widgets", "wid"), filepath.Join(util.DataDir, "widgets", "wid"),
		filepath.Join(util.IconsPath, "ico"), filepath.Join(util.IconsPath, "ico"),
		filepath.Join(util.DataDir, "templates", "tmpl"), filepath.Join(util.DataDir, "templates", "tmpl"),
		filepath.Join(util.ThemesPath, "thm"), filepath.Join(util.ThemesPath, "thm"),
	}
	for i, call := range recorder.calls {
		if !strings.Contains(call, wantTargets[i]) {
			t.Fatalf("call %d target mismatch: %s", i, call)
		}
	}
	if Conf.Appearance.Icon != "ico" || Conf.Appearance.ThemeDark != "thm" || Conf.Appearance.Mode != 1 || Conf.Appearance.ModeOS {
		t.Fatal("appearance effects missing", Conf.Appearance)
	}
	if _, err := installBazaarWidgetContext(ctx, "repo", "hash", "../escape", "", false, ports); err == nil {
		t.Fatal("escaping package name accepted")
	}
}

func TestBatchBazaarMutationPreservesFamilyOrderAndOwnerIdentity(t *testing.T) {
	legacyBackgroundOwnerFixture(t)
	recorder := &bazaarMutationRecorder{}
	ports := mutationPorts(recorder)
	pkg := func(name string) *bazaar.Package {
		return &bazaar.Package{Name: name, RepoURL: "repo", RepoHash: "hash", Outdated: true}
	}
	scan := legacyBazaarScanPorts{
		plugins: func(context.Context, string, string) ([]*bazaar.Plugin, error) {
			return []*bazaar.Plugin{{Package: pkg("plug")}}, nil
		},
		widgets: func(context.Context, string) ([]*bazaar.Widget, error) {
			return []*bazaar.Widget{{Package: pkg("wid")}}, nil
		},
		icons: func(context.Context, string) ([]*bazaar.Icon, error) {
			return []*bazaar.Icon{{Package: pkg("ico")}}, nil
		},
		themes: func(context.Context, string) ([]*bazaar.Theme, error) {
			return []*bazaar.Theme{{Package: pkg("thm")}}, nil
		},
		templates: func(context.Context, string) ([]*bazaar.Template, error) {
			return []*bazaar.Template{{Package: pkg("tmpl")}}, nil
		},
	}
	if err := batchUpdateBazaarPackagesContext(context.Background(), "desktop", scan, ports); err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, call := range recorder.calls {
		kinds = append(kinds, strings.Split(call, ":")[0])
	}
	want := []string{"install-plugin", "install-widget", "install-icon", "install-template", "install-theme"}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatal(kinds)
	}
}

func TestBatchBazaarMutationStopsOnFirstFailure(t *testing.T) {
	legacyBackgroundOwnerFixture(t)
	recorder := &bazaarMutationRecorder{}
	ports := mutationPorts(recorder)
	wantErr := errors.New("widget failed")
	ports.installWidget = func(ctx context.Context, _, _, target, systemID string, _ ...bazaar.BackgroundScheduler) error {
		recorder.add("install-widget", target, systemID)
		return wantErr
	}
	pkg := func(name string) *bazaar.Package {
		return &bazaar.Package{Name: name, RepoURL: "repo", RepoHash: "hash", Outdated: true}
	}
	scan := legacyBazaarScanPorts{
		plugins: func(context.Context, string, string) ([]*bazaar.Plugin, error) {
			return []*bazaar.Plugin{{Package: pkg("plug")}}, nil
		},
		widgets: func(context.Context, string) ([]*bazaar.Widget, error) {
			return []*bazaar.Widget{{Package: pkg("wid")}}, nil
		},
		icons: func(context.Context, string) ([]*bazaar.Icon, error) {
			return []*bazaar.Icon{{Package: pkg("ico")}}, nil
		},
		themes: func(context.Context, string) ([]*bazaar.Theme, error) {
			return []*bazaar.Theme{{Package: pkg("thm")}}, nil
		},
		templates: func(context.Context, string) ([]*bazaar.Template, error) {
			return []*bazaar.Template{{Package: pkg("tmpl")}}, nil
		},
	}
	if err := batchUpdateBazaarPackagesContext(context.Background(), "desktop", scan, ports); !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	var kinds []string
	for _, call := range recorder.calls {
		kinds = append(kinds, strings.Split(call, ":")[0])
	}
	if want := []string{"install-plugin", "install-widget"}; !reflect.DeepEqual(kinds, want) {
		t.Fatal(kinds)
	}
}

func TestBazaarMutationCloseWaitsForActualInstallReturn(t *testing.T) {
	owner := legacyBackgroundOwnerFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	ports := mutationPorts(&bazaarMutationRecorder{})
	ports.installWidget = func(ctx context.Context, _, _, _, _ string, _ ...bazaar.BackgroundScheduler) error {
		close(entered)
		<-release
		return ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		_, err := installBazaarWidgetContext(context.Background(), "repo", "hash", "wid", "", false, ports)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("install did not enter")
	}
	closed := make(chan error, 1)
	go func() { closed <- owner.CloseCloudRepo(context.Background()) }()
	select {
	case err := <-closed:
		t.Fatal("owner closed before install returned", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestBazaarDownloadCountUsesWorkspaceOwner(t *testing.T) {
	owner := legacyBackgroundOwnerFixture(t)
	o, err := captureLegacyBazaarMutationOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	entered, canceled := make(chan struct{}), make(chan struct{})
	if !o.downloadCountScheduler()(func(ctx context.Context) {
		close(entered)
		<-ctx.Done()
		close(canceled)
	}) {
		t.Fatal("count callback was not admitted")
	}
	<-entered
	if err = owner.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("workspace close did not cancel and join count callback")
	}
	if o.downloadCountScheduler()(func(context.Context) {}) {
		t.Fatal("closed workspace admitted a late count callback")
	}
}
