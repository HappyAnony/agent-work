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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/publishsvc"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/conf"
	"wiki-kernel/model"
	"wiki-kernel/server/proxy"
	"wiki-kernel/util"
)

func setEditorReadOnly(c *gin.Context) { applyConfigurationSetting(c, "editorReadOnly", false) }

func setConfSnippet(c *gin.Context) { applyConfigurationSetting(c, "snippet", false) }

func addVirtualBlockRefExclude(c *gin.Context) { applyConfigurationSetting(c, "virtualExclude", false) }

func addVirtualBlockRefInclude(c *gin.Context) { applyConfigurationSetting(c, "virtualInclude", false) }

func refreshVirtualBlockRef(c *gin.Context) { applyConfigurationSetting(c, "virtualRefresh", false) }

func setBazaar(c *gin.Context) { applyConfigurationSetting(c, "bazaar", false) }

func setAI(c *gin.Context) { applyConfigurationSetting(c, "ai", false) }

func setFlashcard(c *gin.Context) { applyConfigurationSetting(c, "flashcard", false) }

func setAccount(c *gin.Context) { applyConfigurationSetting(c, "account", false) }

func setEditor(c *gin.Context) { applyConfigurationSetting(c, "editor", false) }

func setExport(c *gin.Context) { applyConfigurationSetting(c, "export", false) }

func setFiletree(c *gin.Context) { applyConfigurationSetting(c, "fileTree", false) }

func setSearch(c *gin.Context) { applyConfigurationSetting(c, "search", false) }

func setKeymap(c *gin.Context) { applyConfigurationSetting(c, "keymap", true) }

func setAppearance(c *gin.Context) { applyConfigurationSetting(c, "appearance", false) }

func setPublish(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	raw, e := gulu.JSON.MarshalJSON(arg)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
	owner := model.Conf
	var port uint16
	value, e := model.SetLegacyConfigurationWithPorts(c.Request.Context(), "publish", raw, model.SettingPorts{"publish": func(ctx context.Context, p conf.SettingPlan, _ conf.SettingEffect) error {
		if model.Conf != owner {
			return publishsvc.ErrDependency
		}
		var err error
		port, err = proxy.ConfigurePublishService(ctx, p.Data.Publish)
		return err
	}})
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
	ret.Data = map[string]any{"port": port, "publish": json.RawMessage(value)}
}
func getPublish(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	e := model.WithLegacyConfigurationSnapshotContext(c.Request.Context(), func(ctx context.Context, data conf.AppData) error {
		port, e := proxy.ConfigurePublishService(ctx, data.Publish)
		if e != nil {
			return e
		}
		ret.Data = map[string]any{"port": port, "publish": data.Publish}
		return nil
	})
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
	}
}

func getCloudUser(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if !model.IsAdminRoleContext(c) {
		return
	}

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	t := arg["token"]
	var token string
	if nil != t {
		var ok bool
		token, ok = t.(string)
		if !ok {
			ret.Code = -1
			ret.Msg = cloudaccount.ErrResponse.Error()
			return
		}
	}
	user, e := model.RefreshUserContext(c.Request.Context(), token)
	if e != nil && !errors.Is(e, cloudaccount.ErrSession) {
		cloudAccountError(ret, e)
		return
	}
	ret.Data = user
}

func logoutCloudUser(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if e := model.LogoutUserContext(c.Request.Context()); e != nil {
		cloudAccountError(ret, e)
	}
}

func login2faCloudUser(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	token, tok := arg["token"].(string)
	code, cok := arg["code"].(string)
	if !tok || !cok {
		ret.Code = -1
		ret.Msg = cloudaccount.ErrResponse.Error()
		return
	}
	data, err := model.Login2faContext(c.Request.Context(), token, code)
	if err != nil {
		ret.Code = -1
		cloudAccountError(ret, err)
		return
	}
	ret.Data = data
}

func setEmoji(c *gin.Context) { applyConfigurationSetting(c, "emoji", false) }

func applyConfigurationSetting(c *gin.Context, kind string, nested bool) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg := map[string]interface{}{}
	if kind != "virtualRefresh" {
		var ok bool
		arg, ok = util.JsonArg(c, ret)
		if !ok {
			return
		}
	}
	var input any = arg
	if nested {
		input = arg["data"]
	}
	raw, err := gulu.JSON.MarshalJSON(input)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	value, err := model.SetLegacyConfiguration(c.Request.Context(), kind, raw)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		if kind == "export" {
			ret.Data = map[string]interface{}{"closeTimeout": 5000}
		}
		return
	}
	if len(value) > 0 {
		ret.Data = value
	}
}
