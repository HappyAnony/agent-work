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

package api

import (
	"net/http"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func batchUpdatePackage(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	frontend := arg["frontend"].(string)
	if err := model.BatchUpdateBazaarPackagesContext(c.Request.Context(), frontend); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

func getUpdatedPackage(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	frontend := arg["frontend"].(string)
	plugins, widgets, icons, themes, templates, err := model.UpdatedPackagesContext(c.Request.Context(), frontend)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"plugins":   plugins,
		"widgets":   widgets,
		"icons":     icons,
		"themes":    themes,
		"templates": templates,
	}
}

func getBazaarPackageREAME(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	repoURL := arg["repoURL"].(string)
	repoHash := arg["repoHash"].(string)
	packageType := arg["packageType"].(string)
	value, err := model.GetPackageREADMEContext(c.Request.Context(), repoURL, repoHash, packageType)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"html": value,
	}
}

func getBazaarPlugin(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	frontend := arg["frontend"].(string)
	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.BazaarPluginsContext(c.Request.Context(), frontend, keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func getInstalledPlugin(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	frontend := arg["frontend"].(string)
	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.InstalledPluginsContext(c.Request.Context(), frontend, keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func installBazaarPlugin(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	repoURL := arg["repoURL"].(string)
	repoHash := arg["repoHash"].(string)
	packageName := arg["packageName"].(string)
	frontend := arg["frontend"].(string)
	packages, err := model.InstallBazaarPluginContext(c.Request.Context(), repoURL, repoHash, packageName, frontend, keyword)
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"packages": packages,
	}
}

func uninstallBazaarPlugin(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	frontend := arg["frontend"].(string)
	packageName := arg["packageName"].(string)
	packages, err := model.UninstallBazaarPluginContext(c.Request.Context(), packageName, frontend, keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"packages": packages,
	}
}

func getBazaarWidget(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.BazaarWidgetsContext(c.Request.Context(), keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func getInstalledWidget(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.InstalledWidgetsContext(c.Request.Context(), keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func installBazaarWidget(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	repoURL := arg["repoURL"].(string)
	repoHash := arg["repoHash"].(string)
	packageName := arg["packageName"].(string)
	packages, err := model.InstallBazaarWidgetContext(c.Request.Context(), repoURL, repoHash, packageName, keyword)
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"packages": packages,
	}
}

func uninstallBazaarWidget(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	packageName := arg["packageName"].(string)
	packages, err := model.UninstallBazaarWidgetContext(c.Request.Context(), packageName, keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"packages": packages,
	}
}

func getBazaarIcon(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.BazaarIconsContext(c.Request.Context(), keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func getInstalledIcon(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.InstalledIconsContext(c.Request.Context(), keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func installBazaarIcon(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	repoURL := arg["repoURL"].(string)
	repoHash := arg["repoHash"].(string)
	packageName := arg["packageName"].(string)
	result, err := model.InstallBazaarIconContext(c.Request.Context(), repoURL, repoHash, packageName, keyword)
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages":   result.Icons,
		"appearance": result.Appearance,
	}
}

func uninstallBazaarIcon(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	packageName := arg["packageName"].(string)
	result, err := model.UninstallBazaarIconContext(c.Request.Context(), packageName, keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"packages":   result.Icons,
		"appearance": result.Appearance,
	}
}

func getBazaarTemplate(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.BazaarTemplatesContext(c.Request.Context(), keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func getInstalledTemplate(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.InstalledTemplatesContext(c.Request.Context(), keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func installBazaarTemplate(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	repoURL := arg["repoURL"].(string)
	repoHash := arg["repoHash"].(string)
	packageName := arg["packageName"].(string)
	packages, err := model.InstallBazaarTemplateContext(c.Request.Context(), repoURL, repoHash, packageName, keyword)
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"packages": packages,
	}
}

func uninstallBazaarTemplate(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	packageName := arg["packageName"].(string)
	packages, err := model.UninstallBazaarTemplateContext(c.Request.Context(), packageName, keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"packages": packages,
	}
}

func getBazaarTheme(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.BazaarThemesContext(c.Request.Context(), keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func getInstalledTheme(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	value, err := model.InstalledThemesContext(c.Request.Context(), keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"packages": value,
	}
}

func installBazaarTheme(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	repoURL := arg["repoURL"].(string)
	repoHash := arg["repoHash"].(string)
	packageName := arg["packageName"].(string)
	mode := arg["mode"].(float64)
	update := false
	if nil != arg["update"] {
		update = arg["update"].(bool)
	}
	result, err := model.InstallBazaarThemeContext(c.Request.Context(), repoURL, repoHash, packageName, int(mode), update, keyword)
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"packages":   result.Themes,
		"appearance": result.Appearance,
	}
}

func uninstallBazaarTheme(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	var keyword string
	if keywordArg := arg["keyword"]; nil != keywordArg {
		keyword = keywordArg.(string)
	}

	packageName := arg["packageName"].(string)
	result, err := model.UninstallBazaarThemeContext(c.Request.Context(), packageName, keyword)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"packages":   result.Themes,
		"appearance": result.Appearance,
	}
}
