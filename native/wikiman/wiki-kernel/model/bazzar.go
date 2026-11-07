// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"context"
	"strings"

	"github.com/siyuan-note/logging"
	"golang.org/x/sync/errgroup"
	"wiki-kernel/bazaar"
	"wiki-kernel/conf"
	"wiki-kernel/internal/catalogue"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func BatchUpdateBazaarPackages(frontend string) {
	_ = BatchUpdateBazaarPackagesContext(context.Background(), frontend)
}

type legacyBazaarScanOwner struct {
	owner                                   *AppConf
	workspace, data, icons, themes, working string
	icon, lightTheme, darkTheme             string
}

type legacyBazaarScanPorts struct {
	plugins   func(context.Context, string, string) ([]*bazaar.Plugin, error)
	widgets   func(context.Context, string) ([]*bazaar.Widget, error)
	icons     func(context.Context, string) ([]*bazaar.Icon, error)
	themes    func(context.Context, string) ([]*bazaar.Theme, error)
	templates func(context.Context, string) ([]*bazaar.Template, error)
}

func captureLegacyBazaarScanOwner(ctx context.Context) (*legacyBazaarScanOwner, error) {
	if ctx == nil {
		return nil, conf.ErrBusinessUnbound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o := &legacyBazaarScanOwner{owner: Conf, workspace: util.WorkspaceDir, data: util.DataDir, icons: util.IconsPath, themes: util.ThemesPath, working: util.WorkingDir}
	if o.owner == nil {
		return nil, conf.ErrBusinessUnbound
	}
	if err := o.owner.WithSettingSnapshot(ctx, func(v conf.AppData) error {
		if v.Appearance == nil {
			return conf.ErrBusinessUnbound
		}
		o.icon, o.lightTheme, o.darkTheme = v.Appearance.Icon, v.Appearance.ThemeLight, v.Appearance.ThemeDark
		return nil
	}); err != nil {
		return nil, err
	}
	if err := o.guard(ctx); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *legacyBazaarScanOwner) current() bool {
	return o != nil && o.owner != nil && o.owner == Conf && o.workspace == util.WorkspaceDir && o.data == util.DataDir && o.icons == util.IconsPath && o.themes == util.ThemesPath && o.working == util.WorkingDir
}

func (o *legacyBazaarScanOwner) guard(ctx context.Context) error {
	if ctx == nil {
		return conf.ErrBusinessUnbound
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !o.current() {
		return conf.ErrBusinessUnbound
	}
	o.owner.repoMu.Lock()
	closed := o.owner.repoClosed
	o.owner.repoMu.Unlock()
	if closed {
		return conf.ErrBusinessUnbound
	}
	return nil
}

// run admits the whole five-way scan to the workspace task owner. Workspace
// teardown therefore cancels the scan and waits for all workers to return
// before replacing process-wide compatibility paths.
func (o *legacyBazaarScanOwner) run(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return conf.ErrBusinessUnbound
	}
	if err := o.guard(ctx); err != nil {
		return err
	}
	done := make(chan error, 1)
	o.owner.repoMu.Lock()
	if o.owner.repoClosed || !o.current() || ctx.Err() != nil {
		o.owner.repoMu.Unlock()
		return conf.ErrBusinessUnbound
	}
	if o.owner.repoTasks == nil {
		o.owner.repoTasks = taskgroup.Open()
	}
	accepted := o.owner.repoTasks.Go(func(life context.Context) {
		merged, cancel := context.WithCancel(life)
		stop := context.AfterFunc(ctx, cancel)
		defer stop()
		defer cancel()
		done <- fn(merged)
	})
	o.owner.repoMu.Unlock()
	if !accepted {
		return conf.ErrBusinessUnbound
	}
	return <-done
}

func emptyUpdatedPackages() (plugins []*bazaar.Plugin, widgets []*bazaar.Widget, icons []*bazaar.Icon, themes []*bazaar.Theme, templates []*bazaar.Template) {
	return []*bazaar.Plugin{}, []*bazaar.Widget{}, []*bazaar.Icon{}, []*bazaar.Theme{}, []*bazaar.Template{}
}

func UpdatedPackages(frontend string) (plugins []*bazaar.Plugin, widgets []*bazaar.Widget, icons []*bazaar.Icon, themes []*bazaar.Theme, templates []*bazaar.Template) {
	plugins, widgets, icons, themes, templates, _ = UpdatedPackagesContext(context.Background(), frontend)
	return
}

func UpdatedPackagesContext(ctx context.Context, frontend string) (plugins []*bazaar.Plugin, widgets []*bazaar.Widget, icons []*bazaar.Icon, themes []*bazaar.Theme, templates []*bazaar.Template, err error) {
	return updatedPackagesContext(ctx, frontend, defaultBazaarScanPorts())
}

func defaultBazaarScanPorts() legacyBazaarScanPorts {
	return legacyBazaarScanPorts{plugins: InstalledPluginsContext, widgets: InstalledWidgetsContext, icons: InstalledIconsContext, themes: InstalledThemesContext, templates: InstalledTemplatesContext}
}

func updatedPackagesContext(ctx context.Context, frontend string, ports legacyBazaarScanPorts) (plugins []*bazaar.Plugin, widgets []*bazaar.Widget, icons []*bazaar.Icon, themes []*bazaar.Theme, templates []*bazaar.Template, err error) {
	o, err := captureLegacyBazaarScanOwner(ctx)
	if err != nil {
		plugins, widgets, icons, themes, templates = emptyUpdatedPackages()
		return
	}
	err = o.run(ctx, func(life context.Context) error {
		group, workerCtx := errgroup.WithContext(life)
		group.Go(func() error {
			if err := o.guard(workerCtx); err != nil {
				return err
			}
			values, err := ports.plugins(workerCtx, frontend, "")
			if err != nil {
				return err
			}
			for _, plugin := range values {
				if plugin.Outdated {
					plugins = append(plugins, plugin)
				}
			}
			return o.guard(workerCtx)
		})
		group.Go(func() error {
			if err := o.guard(workerCtx); err != nil {
				return err
			}
			values, err := ports.widgets(workerCtx, "")
			if err != nil {
				return err
			}
			for _, widget := range values {
				if widget.Outdated {
					widgets = append(widgets, widget)
				}
			}
			return o.guard(workerCtx)
		})
		group.Go(func() error {
			if err := o.guard(workerCtx); err != nil {
				return err
			}
			values, err := ports.icons(workerCtx, "")
			if err != nil {
				return err
			}
			for _, icon := range values {
				icon.Current = icon.Name == o.icon
				if icon.Outdated {
					icons = append(icons, icon)
				}
			}
			return o.guard(workerCtx)
		})
		group.Go(func() error {
			if err := o.guard(workerCtx); err != nil {
				return err
			}
			values, err := ports.themes(workerCtx, "")
			if err != nil {
				return err
			}
			for _, theme := range values {
				theme.Current = theme.Name == o.lightTheme || theme.Name == o.darkTheme
				if theme.Outdated {
					themes = append(themes, theme)
				}
			}
			return o.guard(workerCtx)
		})
		group.Go(func() error {
			if err := o.guard(workerCtx); err != nil {
				return err
			}
			values, err := ports.templates(workerCtx, "")
			if err != nil {
				return err
			}
			for _, template := range values {
				if template.Outdated {
					templates = append(templates, template)
				}
			}
			return o.guard(workerCtx)
		})
		return group.Wait()
	})
	if err != nil {
		plugins, widgets, icons, themes, templates = emptyUpdatedPackages()
		return
	}
	if plugins == nil {
		plugins = []*bazaar.Plugin{}
	}
	if widgets == nil {
		widgets = []*bazaar.Widget{}
	}
	if icons == nil {
		icons = []*bazaar.Icon{}
	}
	if themes == nil {
		themes = []*bazaar.Theme{}
	}
	if templates == nil {
		templates = []*bazaar.Template{}
	}
	return
}

func GetPackageREADME(repo, hash, kind string) string {
	out, e := GetPackageREADMEContext(context.Background(), repo, hash, kind)
	if e != nil {
		logging.LogErrorf("catalogue readme: %s", e)
	}
	return out
}

func BazaarPlugins(frontend, keyword string) []*bazaar.Plugin {
	out, e := BazaarPluginsContext(context.Background(), frontend, keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func filterPlugins(plugins []*bazaar.Plugin, keyword string) (ret []*bazaar.Plugin) {
	ret = []*bazaar.Plugin{}
	keywords := getSearchKeywords(keyword)
	for _, plugin := range plugins {
		if matchPackage(keywords, plugin.Package) {
			ret = append(ret, plugin)
		}
	}
	return
}

func InstalledPlugins(frontend, keyword string) []*bazaar.Plugin {
	out, e := InstalledPluginsContext(context.Background(), frontend, keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func InstallBazaarPlugin(repoURL, repoHash, pluginName string) error {
	_, err := installBazaarPluginContext(context.Background(), repoURL, repoHash, pluginName, "", "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func UninstallBazaarPlugin(pluginName, frontend string) error {
	_, err := uninstallBazaarPluginContext(context.Background(), pluginName, frontend, "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func BazaarWidgets(keyword string) []*bazaar.Widget {
	out, e := BazaarWidgetsContext(context.Background(), keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func filterWidgets(widgets []*bazaar.Widget, keyword string) (ret []*bazaar.Widget) {
	ret = []*bazaar.Widget{}
	keywords := getSearchKeywords(keyword)
	for _, w := range widgets {
		if matchPackage(keywords, w.Package) {
			ret = append(ret, w)
		}
	}
	return
}

func InstalledWidgets(keyword string) []*bazaar.Widget {
	out, e := InstalledWidgetsContext(context.Background(), keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func InstallBazaarWidget(repoURL, repoHash, widgetName string) error {
	_, err := installBazaarWidgetContext(context.Background(), repoURL, repoHash, widgetName, "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func UninstallBazaarWidget(widgetName string) error {
	_, err := uninstallBazaarWidgetContext(context.Background(), widgetName, "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func BazaarIcons(keyword string) []*bazaar.Icon {
	out, e := BazaarIconsContext(context.Background(), keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func filterIcons(icons []*bazaar.Icon, keyword string) (ret []*bazaar.Icon) {
	ret = []*bazaar.Icon{}
	keywords := getSearchKeywords(keyword)
	for _, i := range icons {
		if matchPackage(keywords, i.Package) {
			ret = append(ret, i)
		}
	}
	return
}

func InstalledIcons(keyword string) []*bazaar.Icon {
	out, e := InstalledIconsContext(context.Background(), keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func InstallBazaarIcon(repoURL, repoHash, iconName string) error {
	_, err := installBazaarIconContext(context.Background(), repoURL, repoHash, iconName, "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func UninstallBazaarIcon(iconName string) error {
	_, err := uninstallBazaarIconContext(context.Background(), iconName, "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func BazaarThemes(keyword string) []*bazaar.Theme {
	out, e := BazaarThemesContext(context.Background(), keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func filterThemes(themes []*bazaar.Theme, keyword string) (ret []*bazaar.Theme) {
	ret = []*bazaar.Theme{}
	keywords := getSearchKeywords(keyword)
	for _, t := range themes {
		if matchPackage(keywords, t.Package) {
			ret = append(ret, t)
		}
	}
	return
}

func InstalledThemes(keyword string) []*bazaar.Theme {
	out, e := InstalledThemesContext(context.Background(), keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func InstallBazaarTheme(repoURL, repoHash, themeName string, mode int, update bool) error {
	_, err := installBazaarThemeContext(context.Background(), repoURL, repoHash, themeName, mode, update, "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func UninstallBazaarTheme(themeName string) error {
	_, err := uninstallBazaarThemeContext(context.Background(), themeName, "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func BazaarTemplates(keyword string) []*bazaar.Template {
	out, e := BazaarTemplatesContext(context.Background(), keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func filterTemplates(templates []*bazaar.Template, keyword string) (ret []*bazaar.Template) {
	ret = []*bazaar.Template{}
	keywords := getSearchKeywords(keyword)
	for _, t := range templates {
		if matchPackage(keywords, t.Package) {
			ret = append(ret, t)
		}
	}
	return
}

func InstalledTemplates(keyword string) []*bazaar.Template {
	out, e := InstalledTemplatesContext(context.Background(), keyword)
	if e != nil {
		logging.LogErrorf("catalogue query: %s", e)
	}
	return out
}

func InstallBazaarTemplate(repoURL, repoHash, templateName string) error {
	_, err := installBazaarTemplateContext(context.Background(), repoURL, repoHash, templateName, "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func UninstallBazaarTemplate(templateName string) error {
	_, err := uninstallBazaarTemplateContext(context.Background(), templateName, "", false, defaultLegacyBazaarMutationPorts)
	return err
}

func matchPackage(keywords []string, p *bazaar.Package) bool {
	return catalogue.Match(strings.Join(keywords, " "), p)
}

func getSearchKeywords(query string) (ret []string) {
	query = strings.TrimSpace(query)
	if "" == query {
		return
	}

	keywords := strings.Split(query, " ")
	for _, k := range keywords {
		if "" != k {
			ret = append(ret, strings.ToLower(k))
		}
	}
	return
}
