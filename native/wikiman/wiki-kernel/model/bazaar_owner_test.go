package model

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"wiki-kernel/bazaar"
	"wiki-kernel/conf"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

func legacyBackgroundOwnerFixture(t *testing.T) *AppConf {
	t.Helper()
	root := t.TempDir()
	owner := settingOwner(t, filepath.Join(root, "conf.json"), true)
	owner.Appearance = conf.NewAppearance()
	owner.System = conf.NewSystemForDevice("owned-system", "owned", "test")
	owner.Lang = "en_US"
	owner.business = conf.NewBusiness(nil, map[string]map[int]string{
		"en_US": {
			46: "install %s failed: %s", 47: "uninstall failed: %s", 69: "installed",
			235: "updating %d/%d", 236: "updated %d/%d %s", 237: "updated %d packages", 238: "update %s failed",
		},
	}, nil)
	owner.Editor.GenerateHistoryInterval = 1
	owner.Editor.HistoryRetentionDays = 7
	oldOwner := Conf
	oldWorkspace, oldData, oldHistory := util.WorkspaceDir, util.DataDir, util.HistoryDir
	oldTemp, oldWorking := util.TempDir, util.WorkingDir
	oldIcons, oldThemes, oldAppearance := util.IconsPath, util.ThemesPath, util.AppearancePath
	Conf = owner
	util.WorkspaceDir = root
	util.DataDir = filepath.Join(root, "data")
	util.HistoryDir = filepath.Join(root, "history")
	util.TempDir = filepath.Join(root, "temp")
	util.WorkingDir = filepath.Join(root, "working")
	util.IconsPath = filepath.Join(root, "icons")
	util.ThemesPath = filepath.Join(root, "themes")
	util.AppearancePath = filepath.Join(root, "appearance")
	util.IsExiting.Store(false)
	_ = task.CloseLegacyTasks(context.Background())
	task.OpenLegacyTasks()
	t.Cleanup(func() {
		_ = task.CloseLegacyTasks(context.Background())
		// The fixture must honor the same barrier as a workspace switch. Dropping
		// repoTasks would detach accepted delayed callbacks from their owner.
		if err := owner.CloseCloudRepo(context.Background()); err != nil {
			t.Error(err)
		}
		Conf = oldOwner
		util.WorkspaceDir, util.DataDir, util.HistoryDir = oldWorkspace, oldData, oldHistory
		util.TempDir, util.WorkingDir = oldTemp, oldWorking
		util.IconsPath, util.ThemesPath, util.AppearancePath = oldIcons, oldThemes, oldAppearance
	})
	return owner
}

func TestUpdatedPackagesContextRunsFiveWorkersAndFreezesAppearance(t *testing.T) {
	legacyBackgroundOwnerFixture(t)
	Conf.Appearance.Icon = "owned-icon"
	Conf.Appearance.ThemeLight = "owned-light"
	Conf.Appearance.ThemeDark = "owned-dark"
	entered := make(chan struct{}, 5)
	release := make(chan struct{})
	wait := func() { entered <- struct{}{}; <-release }
	ports := legacyBazaarScanPorts{
		plugins: func(context.Context, string, string) ([]*bazaar.Plugin, error) {
			wait()
			return []*bazaar.Plugin{{Package: &bazaar.Package{Name: "plugin", Outdated: true}}, {Package: &bazaar.Package{Name: "current"}}}, nil
		},
		widgets: func(context.Context, string) ([]*bazaar.Widget, error) {
			wait()
			return []*bazaar.Widget{{Package: &bazaar.Package{Name: "widget", Outdated: true}}}, nil
		},
		icons: func(context.Context, string) ([]*bazaar.Icon, error) {
			wait()
			return []*bazaar.Icon{{Package: &bazaar.Package{Name: "owned-icon", Outdated: true}}}, nil
		},
		themes: func(context.Context, string) ([]*bazaar.Theme, error) {
			wait()
			return []*bazaar.Theme{{Package: &bazaar.Package{Name: "owned-dark", Outdated: true}}}, nil
		},
		templates: func(context.Context, string) ([]*bazaar.Template, error) {
			wait()
			return []*bazaar.Template{{Package: &bazaar.Package{Name: "template", Outdated: true}}}, nil
		},
	}
	type result struct {
		plugins   []*bazaar.Plugin
		widgets   []*bazaar.Widget
		icons     []*bazaar.Icon
		themes    []*bazaar.Theme
		templates []*bazaar.Template
		err       error
	}
	done := make(chan result, 1)
	go func() {
		p, w, i, th, tm, err := updatedPackagesContext(context.Background(), "desktop", ports)
		done <- result{p, w, i, th, tm, err}
	}()
	for n := 0; n < 5; n++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("workers did not run concurrently")
		}
	}
	Conf.Appearance.Icon = "replacement-icon"
	Conf.Appearance.ThemeDark = "replacement-theme"
	close(release)
	r := <-done
	if r.err != nil || len(r.plugins) != 1 || len(r.widgets) != 1 || len(r.icons) != 1 || len(r.themes) != 1 || len(r.templates) != 1 {
		t.Fatal(r.err, len(r.plugins), len(r.widgets), len(r.icons), len(r.themes), len(r.templates))
	}
	if !r.icons[0].Current || !r.themes[0].Current {
		t.Fatal("appearance was not frozen at admission")
	}
}

func TestUpdatedPackagesContextCloseCancelsAndJoinsWorkers(t *testing.T) {
	owner := legacyBackgroundOwnerFixture(t)
	entered := make(chan struct{}, 5)
	release := make(chan struct{})
	wait := func() { entered <- struct{}{}; <-release }
	ports := legacyBazaarScanPorts{
		plugins:   func(context.Context, string, string) ([]*bazaar.Plugin, error) { wait(); return nil, nil },
		widgets:   func(context.Context, string) ([]*bazaar.Widget, error) { wait(); return nil, nil },
		icons:     func(context.Context, string) ([]*bazaar.Icon, error) { wait(); return nil, nil },
		themes:    func(context.Context, string) ([]*bazaar.Theme, error) { wait(); return nil, nil },
		templates: func(context.Context, string) ([]*bazaar.Template, error) { wait(); return nil, nil },
	}
	scanDone := make(chan error, 1)
	go func() {
		_, _, _, _, _, err := updatedPackagesContext(context.Background(), "desktop", ports)
		scanDone <- err
	}()
	for n := 0; n < 5; n++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("worker did not enter")
		}
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- owner.CloseCloudRepo(context.Background()) }()
	select {
	case err := <-closeDone:
		t.Fatal("owner closed before live workers returned", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-scanDone; !errors.Is(err, context.Canceled) {
		t.Fatal("scan did not report owner cancellation", err)
	}
}
