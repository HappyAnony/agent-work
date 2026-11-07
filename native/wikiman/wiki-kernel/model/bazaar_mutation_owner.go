// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/88250/gulu"
	"github.com/emirpasic/gods/sets/hashset"
	"github.com/siyuan-note/logging"
	"wiki-kernel/bazaar"
	"wiki-kernel/conf"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

type legacyBazaarMutationOwner struct {
	scan             *legacyBazaarScanOwner
	temp, appearance string
	systemID, lang   string
	cloudRegion      int
}

type legacyBazaarMutationPorts struct {
	installPlugin, installWidget, installIcon, installTheme, installTemplate           func(context.Context, string, string, string, string, ...bazaar.BackgroundScheduler) error
	uninstallPlugin, uninstallWidget, uninstallIcon, uninstallTheme, uninstallTemplate func(context.Context, string) error
	initAppearance                                                                     func(context.Context) error
	closeThemeWatchers                                                                 func()
}

var defaultLegacyBazaarMutationPorts = legacyBazaarMutationPorts{
	installPlugin: bazaar.InstallPluginContext, installWidget: bazaar.InstallWidgetContext,
	installIcon: bazaar.InstallIconContext, installTheme: bazaar.InstallThemeContext,
	installTemplate: bazaar.InstallTemplateContext,
	uninstallPlugin: bazaar.UninstallPluginContext, uninstallWidget: bazaar.UninstallWidgetContext,
	uninstallIcon: bazaar.UninstallIconContext, uninstallTheme: bazaar.UninstallThemeContext,
	uninstallTemplate: bazaar.UninstallTemplateContext,
	initAppearance:    InitAppearanceContext, closeThemeWatchers: closeThemeWatchers,
}

type BazaarAppearanceMutationResult struct {
	Icons      []*bazaar.Icon
	Themes     []*bazaar.Theme
	Appearance *conf.Appearance
}

func captureLegacyBazaarMutationOwner(ctx context.Context) (*legacyBazaarMutationOwner, error) {
	scan, err := captureLegacyBazaarScanOwner(ctx)
	if err != nil {
		return nil, err
	}
	o := &legacyBazaarMutationOwner{scan: scan, temp: util.TempDir, appearance: util.AppearancePath}
	if err = scan.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.System == nil || v.Appearance == nil {
			return conf.ErrBusinessUnbound
		}
		o.systemID, o.lang, o.cloudRegion = v.System.ID, v.Lang, v.CloudRegion
		return nil
	}); err != nil {
		return nil, err
	}
	if err = o.guard(ctx); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *legacyBazaarMutationOwner) current() bool {
	return o != nil && o.scan != nil && o.scan.current() && o.temp == util.TempDir && o.appearance == util.AppearancePath
}

func (o *legacyBazaarMutationOwner) guard(ctx context.Context) error {
	if o == nil || !o.current() {
		return conf.ErrBusinessUnbound
	}
	return o.scan.guard(ctx)
}

func (o *legacyBazaarMutationOwner) run(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return conf.ErrBusinessUnbound
	}
	return o.scan.run(ctx, func(life context.Context) error {
		if err := o.guard(life); err != nil {
			return err
		}
		if err := fn(life); err != nil {
			return err
		}
		return o.guard(life)
	})
}

func (o *legacyBazaarMutationOwner) language(code int) string {
	if o == nil || o.scan == nil || o.scan.owner == nil || o.scan.owner.business == nil {
		return ""
	}
	return o.scan.owner.business.Language(o.lang, code, true, o.cloudRegion)
}

func (o *legacyBazaarMutationOwner) publish(ctx context.Context, fn func()) bool {
	if fn == nil || ctx == nil || ctx.Err() != nil || !o.current() {
		return false
	}
	o.scan.owner.repoMu.Lock()
	defer o.scan.owner.repoMu.Unlock()
	if o.scan.owner.repoClosed || ctx.Err() != nil || !o.current() {
		return false
	}
	fn()
	return true
}

func (o *legacyBazaarMutationOwner) downloadCountScheduler() bazaar.BackgroundScheduler {
	return func(fn func(context.Context)) bool {
		if fn == nil || o == nil || o.scan == nil || o.scan.owner == nil {
			return false
		}
		return o.scan.owner.startRepoBackground(func(life context.Context) {
			if o.guard(life) == nil {
				fn(life)
			}
		})
	}
}

func (o *legacyBazaarMutationOwner) updateAppearance(ctx context.Context, update func(*conf.Appearance)) error {
	if update == nil {
		return conf.ErrBusinessUnbound
	}
	return o.scan.owner.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		if err := o.guard(ctx); err != nil {
			return err
		}
		if data.Appearance == nil {
			return conf.ErrBusinessUnbound
		}
		update(data.Appearance)
		o.scan.owner.repoMu.Lock()
		defer o.scan.owner.repoMu.Unlock()
		if o.scan.owner.repoClosed || ctx.Err() != nil || !o.current() {
			if err := ctx.Err(); err != nil {
				return err
			}
			return conf.ErrBusinessUnbound
		}
		o.scan.owner.m.Lock()
		err := o.scan.owner.saveDataLocked(data)
		if err == nil {
			o.scan.owner.AppData = data
		}
		o.scan.owner.m.Unlock()
		return err
	})
}

func (o *legacyBazaarMutationOwner) appearanceSnapshot(ctx context.Context) (*conf.Appearance, error) {
	var ret *conf.Appearance
	err := o.scan.owner.WithSettingSnapshot(ctx, func(data conf.AppData) error {
		if err := o.guard(ctx); err != nil {
			return err
		}
		if data.Appearance == nil {
			return conf.ErrBusinessUnbound
		}
		ret = data.Appearance
		return nil
	})
	return ret, err
}

func validBazaarPackageName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name
}

func (o *legacyBazaarMutationOwner) target(kind, name string) (string, error) {
	if !validBazaarPackageName(name) {
		return "", errors.New("invalid bazaar package name")
	}
	switch kind {
	case "plugin":
		return filepath.Join(o.scan.data, "plugins", name), nil
	case "widget":
		return filepath.Join(o.scan.data, "widgets", name), nil
	case "icon":
		return filepath.Join(o.scan.icons, name), nil
	case "theme":
		return filepath.Join(o.scan.themes, name), nil
	case "template":
		return filepath.Join(o.scan.data, "templates", name), nil
	default:
		return "", conf.ErrBusinessUnbound
	}
}

func (o *legacyBazaarMutationOwner) installError(name string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New(fmt.Sprintf(o.language(46), name, err))
}

func (o *legacyBazaarMutationOwner) uninstallError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New(fmt.Sprintf(o.language(47), err.Error()))
}

func BatchUpdateBazaarPackagesContext(ctx context.Context, frontend string) error {
	return batchUpdateBazaarPackagesContext(ctx, frontend, defaultBazaarScanPorts(), defaultLegacyBazaarMutationPorts)
}

func batchUpdateBazaarPackagesContext(ctx context.Context, frontend string, scanPorts legacyBazaarScanPorts, ports legacyBazaarMutationPorts) error {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return err
	}
	return o.run(ctx, func(life context.Context) error {
		uiOwner, _ := captureLegacyDelayedUIOwner(life)
		plugins, widgets, icons, themes, templates, err := updatedPackagesContext(life, frontend, scanPorts)
		if err != nil {
			return err
		}
		total := len(plugins) + len(widgets) + len(icons) + len(themes) + len(templates)
		if total < 1 {
			return nil
		}
		o.publish(life, func() { util.PushEndlessProgress(fmt.Sprintf(o.language(235), 1, total)) })
		defer o.publish(context.WithoutCancel(life), util.PushClearProgress)
		count := 1
		progress := func(name string) {
			count++
			o.publish(life, func() { util.PushEndlessProgress(fmt.Sprintf(o.language(236), count, total, name)) })
		}
		failed := func(kind, name string, cause error) error {
			logging.LogErrorf("update %s [%s] failed: %s", kind, name, cause)
			if !errors.Is(cause, context.Canceled) && !errors.Is(cause, context.DeadlineExceeded) {
				o.publish(context.WithoutCancel(life), func() { util.PushErrMsg(fmt.Sprintf(o.language(238), name), 5000) })
			}
			return cause
		}
		for _, plugin := range plugins {
			target, targetErr := o.target("plugin", plugin.Name)
			if targetErr != nil {
				return targetErr
			}
			if err = ports.installPlugin(life, plugin.RepoURL, plugin.RepoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
				return failed("plugin", plugin.Name, err)
			}
			progress(plugin.Name)
		}
		for _, widget := range widgets {
			target, targetErr := o.target("widget", widget.Name)
			if targetErr != nil {
				return targetErr
			}
			if err = ports.installWidget(life, widget.RepoURL, widget.RepoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
				return failed("widget", widget.Name, err)
			}
			progress(widget.Name)
		}
		for _, icon := range icons {
			target, targetErr := o.target("icon", icon.Name)
			if targetErr != nil {
				return targetErr
			}
			if err = ports.installIcon(life, icon.RepoURL, icon.RepoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
				return failed("icon", icon.Name, err)
			}
			progress(icon.Name)
		}
		for _, template := range templates {
			target, targetErr := o.target("template", template.Name)
			if targetErr != nil {
				return targetErr
			}
			if err = ports.installTemplate(life, template.RepoURL, template.RepoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
				return failed("template", template.Name, err)
			}
			progress(template.Name)
		}
		for _, theme := range themes {
			target, targetErr := o.target("theme", theme.Name)
			if targetErr != nil {
				return targetErr
			}
			if err = ports.installTheme(life, theme.RepoURL, theme.RepoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
				return failed("theme", theme.Name, err)
			}
			progress(theme.Name)
		}
		if !o.publish(life, util.ReloadUI) {
			return conf.ErrBusinessUnbound
		}
		if uiOwner != nil {
			message := fmt.Sprintf(uiOwner.language(237, false), total)
			if err = uiOwner.schedule(context.WithoutCancel(life), task.PushMsg, 3*time.Second, func() { util.PushMsg(message, 5000) }, message, 5000); err != nil {
				return err
			}
		}
		return nil
	})
}

func InstallBazaarPluginContext(ctx context.Context, repoURL, repoHash, name, frontend, keyword string) ([]*bazaar.Plugin, error) {
	return installBazaarPluginContext(ctx, repoURL, repoHash, name, frontend, keyword, true, defaultLegacyBazaarMutationPorts)
}

func installBazaarPluginContext(ctx context.Context, repoURL, repoHash, name, frontend, keyword string, project bool, ports legacyBazaarMutationPorts) (packages []*bazaar.Plugin, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return nil, err
	}
	target, err := o.target("plugin", name)
	if err != nil {
		return nil, err
	}
	err = o.run(ctx, func(life context.Context) error {
		if err := ports.installPlugin(life, repoURL, repoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
			return o.installError(name, err)
		}
		o.publish(life, func() { util.PushMsg(o.language(69), 3000) })
		if project {
			packages, err = BazaarPluginsContext(life, frontend, keyword)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return packages, err
}

func UninstallBazaarPluginContext(ctx context.Context, name, frontend, keyword string) ([]*bazaar.Plugin, error) {
	return uninstallBazaarPluginContext(ctx, name, frontend, keyword, true, defaultLegacyBazaarMutationPorts)
}

func uninstallBazaarPluginContext(ctx context.Context, name, frontend, keyword string, project bool, ports legacyBazaarMutationPorts) (packages []*bazaar.Plugin, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return nil, err
	}
	target, err := o.target("plugin", name)
	if err != nil {
		return nil, err
	}
	err = o.run(ctx, func(life context.Context) error {
		if err := ports.uninstallPlugin(life, target); err != nil {
			return o.uninstallError(err)
		}
		petals, err := GetPetalsContext(life)
		if err != nil {
			return err
		}
		var retained []*Petal
		for _, petal := range petals {
			if petal.Name != name {
				retained = append(retained, petal)
			}
		}
		if err = SavePetalsContext(life, retained); err != nil {
			return err
		}
		if err = PushReloadPluginContext(life, nil, nil, hashset.New(name), ""); err != nil {
			return err
		}
		if project {
			packages, err = BazaarPluginsContext(life, frontend, keyword)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return packages, err
}

func InstallBazaarWidgetContext(ctx context.Context, repoURL, repoHash, name, keyword string) ([]*bazaar.Widget, error) {
	return installBazaarWidgetContext(ctx, repoURL, repoHash, name, keyword, true, defaultLegacyBazaarMutationPorts)
}

func installBazaarWidgetContext(ctx context.Context, repoURL, repoHash, name, keyword string, project bool, ports legacyBazaarMutationPorts) (packages []*bazaar.Widget, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return nil, err
	}
	target, err := o.target("widget", name)
	if err != nil {
		return nil, err
	}
	err = o.run(ctx, func(life context.Context) error {
		if err := ports.installWidget(life, repoURL, repoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
			return o.installError(name, err)
		}
		o.publish(life, func() { util.PushMsg(o.language(69), 3000) })
		if project {
			packages, err = BazaarWidgetsContext(life, keyword)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return packages, err
}

func UninstallBazaarWidgetContext(ctx context.Context, name, keyword string) ([]*bazaar.Widget, error) {
	return uninstallBazaarWidgetContext(ctx, name, keyword, true, defaultLegacyBazaarMutationPorts)
}

func uninstallBazaarWidgetContext(ctx context.Context, name, keyword string, project bool, ports legacyBazaarMutationPorts) (packages []*bazaar.Widget, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return nil, err
	}
	target, err := o.target("widget", name)
	if err != nil {
		return nil, err
	}
	err = o.run(ctx, func(life context.Context) error {
		if err := ports.uninstallWidget(life, target); err != nil {
			return o.uninstallError(err)
		}
		if project {
			packages, err = BazaarWidgetsContext(life, keyword)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return packages, err
}

func InstallBazaarIconContext(ctx context.Context, repoURL, repoHash, name, keyword string) (BazaarAppearanceMutationResult, error) {
	return installBazaarIconContext(ctx, repoURL, repoHash, name, keyword, true, defaultLegacyBazaarMutationPorts)
}

func installBazaarIconContext(ctx context.Context, repoURL, repoHash, name, keyword string, project bool, ports legacyBazaarMutationPorts) (result BazaarAppearanceMutationResult, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return result, err
	}
	target, err := o.target("icon", name)
	if err != nil {
		return result, err
	}
	err = o.run(ctx, func(life context.Context) error {
		if err := ports.installIcon(life, repoURL, repoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
			return o.installError(name, err)
		}
		if err := o.updateAppearance(life, func(a *conf.Appearance) { a.Icon = name }); err != nil {
			return err
		}
		if err := ports.initAppearance(life); err != nil {
			return err
		}
		o.publish(life, func() {
			util.BroadcastByType("main", "setAppearance", 0, "", o.scan.owner.Appearance)
			util.PushMsg(o.language(69), 3000)
		})
		if project {
			result.Icons, err = BazaarIconsContext(life, keyword)
			if err != nil {
				return err
			}
		}
		result.Appearance, err = o.appearanceSnapshot(life)
		return err
	})
	return result, err
}

func UninstallBazaarIconContext(ctx context.Context, name, keyword string) (BazaarAppearanceMutationResult, error) {
	return uninstallBazaarIconContext(ctx, name, keyword, true, defaultLegacyBazaarMutationPorts)
}

func uninstallBazaarIconContext(ctx context.Context, name, keyword string, project bool, ports legacyBazaarMutationPorts) (result BazaarAppearanceMutationResult, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return result, err
	}
	target, err := o.target("icon", name)
	if err != nil {
		return result, err
	}
	err = o.run(ctx, func(life context.Context) error {
		if err := ports.uninstallIcon(life, target); err != nil {
			return o.uninstallError(err)
		}
		if err := ports.initAppearance(life); err != nil {
			return err
		}
		if project {
			result.Icons, err = BazaarIconsContext(life, keyword)
			if err != nil {
				return err
			}
		}
		result.Appearance, err = o.appearanceSnapshot(life)
		return err
	})
	return result, err
}

func InstallBazaarTemplateContext(ctx context.Context, repoURL, repoHash, name, keyword string) ([]*bazaar.Template, error) {
	return installBazaarTemplateContext(ctx, repoURL, repoHash, name, keyword, true, defaultLegacyBazaarMutationPorts)
}

func installBazaarTemplateContext(ctx context.Context, repoURL, repoHash, name, keyword string, project bool, ports legacyBazaarMutationPorts) (packages []*bazaar.Template, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return nil, err
	}
	target, err := o.target("template", name)
	if err != nil {
		return nil, err
	}
	err = o.run(ctx, func(life context.Context) error {
		if err := ports.installTemplate(life, repoURL, repoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
			return o.installError(name, err)
		}
		o.publish(life, func() { util.PushMsg(o.language(69), 3000) })
		if project {
			packages, err = BazaarTemplatesContext(life, keyword)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return packages, err
}

func UninstallBazaarTemplateContext(ctx context.Context, name, keyword string) ([]*bazaar.Template, error) {
	return uninstallBazaarTemplateContext(ctx, name, keyword, true, defaultLegacyBazaarMutationPorts)
}

func uninstallBazaarTemplateContext(ctx context.Context, name, keyword string, project bool, ports legacyBazaarMutationPorts) (packages []*bazaar.Template, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return nil, err
	}
	target, err := o.target("template", name)
	if err != nil {
		return nil, err
	}
	err = o.run(ctx, func(life context.Context) error {
		if err := ports.uninstallTemplate(life, target); err != nil {
			return o.uninstallError(err)
		}
		if project {
			packages, err = BazaarTemplatesContext(life, keyword)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return packages, err
}

func InstallBazaarThemeContext(ctx context.Context, repoURL, repoHash, name string, mode int, update bool, keyword string) (BazaarAppearanceMutationResult, error) {
	return installBazaarThemeContext(ctx, repoURL, repoHash, name, mode, update, keyword, true, defaultLegacyBazaarMutationPorts)
}

func installBazaarThemeContext(ctx context.Context, repoURL, repoHash, name string, mode int, update bool, keyword string, project bool, ports legacyBazaarMutationPorts) (result BazaarAppearanceMutationResult, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return result, err
	}
	target, err := o.target("theme", name)
	if err != nil {
		return result, err
	}
	err = o.run(ctx, func(life context.Context) error {
		ports.closeThemeWatchers()
		if err := ports.installTheme(life, repoURL, repoHash, target, o.systemID, o.downloadCountScheduler()); err != nil {
			return o.installError(name, err)
		}
		if err := o.updateAppearance(life, func(a *conf.Appearance) {
			if !update {
				if mode == 0 {
					a.ThemeLight = name
				} else {
					a.ThemeDark = name
				}
				a.Mode = mode
				a.ThemeJS = gulu.File.IsExist(filepath.Join(target, "theme.js"))
			}
			a.ModeOS = false
		}); err != nil {
			return err
		}
		if err := ports.initAppearance(life); err != nil {
			return err
		}
		o.publish(life, func() {
			util.BroadcastByType("main", "setAppearance", 0, "", o.scan.owner.Appearance)
			util.PushMsg(o.language(69), 3000)
		})
		if project {
			result.Themes, err = BazaarThemesContext(life, keyword)
			if err != nil {
				return err
			}
		}
		result.Appearance, err = o.appearanceSnapshot(life)
		return err
	})
	return result, err
}

func UninstallBazaarThemeContext(ctx context.Context, name, keyword string) (BazaarAppearanceMutationResult, error) {
	return uninstallBazaarThemeContext(ctx, name, keyword, true, defaultLegacyBazaarMutationPorts)
}

func uninstallBazaarThemeContext(ctx context.Context, name, keyword string, project bool, ports legacyBazaarMutationPorts) (result BazaarAppearanceMutationResult, err error) {
	o, err := captureLegacyBazaarMutationOwner(ctx)
	if err != nil {
		return result, err
	}
	target, err := o.target("theme", name)
	if err != nil {
		return result, err
	}
	err = o.run(ctx, func(life context.Context) error {
		ports.closeThemeWatchers()
		if err := ports.uninstallTheme(life, target); err != nil {
			return o.uninstallError(err)
		}
		if err := ports.initAppearance(life); err != nil {
			return err
		}
		if project {
			result.Themes, err = BazaarThemesContext(life, keyword)
			if err != nil {
				return err
			}
		}
		result.Appearance, err = o.appearanceSnapshot(life)
		return err
	})
	return result, err
}
