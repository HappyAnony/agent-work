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
	"encoding/json"
	"errors"
	"net/http"
	"wiki-kernel/internal/cloudrepo"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func importSyncProviderWebDAV(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	out, err := model.ImportSyncProviderRequest(c.Request.Context(), c.Request, "syncWebDAV")
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{"webdav": out}
}

func exportSyncProviderWebDAV(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	out, err := model.ExportSettingPackageContext(c.Request.Context(), "syncWebDAV")
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = out
}

func importSyncProviderS3(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	out, err := model.ImportSyncProviderRequest(c.Request.Context(), c.Request, "syncS3")
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{"s3": out}
}

func exportSyncProviderS3(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	out, err := model.ExportSettingPackageContext(c.Request.Context(), "syncS3")
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = out
}

func getSyncInfo(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	state, err := model.SyncStateContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"synced": state.Synced, "stat": state.Stat,
		"kernels": state.Kernels, "kernel": state.Kernel,
	}
}

func getBootSync(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if !model.IsAdminRoleContext(c) {
		return
	}

	state, err := model.SyncStateContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	if state.Enabled && 1 == state.Boot {
		ret.Code = 1
		ret.Msg = state.BootMessage
		return
	}
}

func performSync(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	// Android 端前后台切换时自动触发同步 https://github.com/siyuan-note/siyuan/issues/7122
	var mobileSwitch bool
	if mobileSwitchArg := arg["mobileSwitch"]; nil != mobileSwitchArg {
		mobileSwitch = mobileSwitchArg.(bool)
	}
	state, err := model.SyncStateContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	if mobileSwitch {
		if !state.HasUser || !state.Enabled {
			return
		}
	}

	if 3 != state.Mode {
		if _, err := model.SyncDataContext(c.Request.Context(), "normal", true); err != nil {
			ret.Code = -1
			ret.Msg = err.Error()
		}
		return
	}

	// 云端同步模式支持 `完全手动同步` 模式 https://github.com/siyuan-note/siyuan/issues/7295
	uploadArg := arg["upload"]
	if nil == uploadArg {
		// 必须传入同步方向，未传的话不执行同步
		return
	}

	upload := uploadArg.(bool)
	if upload {
		if _, err := model.SyncDataContext(c.Request.Context(), "upload", true); err != nil {
			ret.Code = -1
			ret.Msg = err.Error()
		}
	} else {
		if _, err := model.SyncDataContext(c.Request.Context(), "download", true); err != nil {
			ret.Code = -1
			ret.Msg = err.Error()
		}
	}
}

func performBootSync(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	if _, err := model.SyncDataContext(c.Request.Context(), "boot", false); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	status, err := model.BootSyncStatusContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Code = status
}

func listCloudSyncDir(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	out, e := model.QueryCloudRepoContext(c.Request.Context(), "repos", 1)
	if e != nil {
		ret.Code = 1
		ret.Msg = e.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
	ret.Data = out.Data
}

func removeCloudSyncDir(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	name, ok := arg["name"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid name"
		return
	}
	out, e := model.ChangeCloudDirectoryContext(c.Request.Context(), "remove", name)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
	ret.Data = out.Active

}

func createCloudSyncDir(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	name, ok := arg["name"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid name"
		return
	}
	out, e := model.ChangeCloudDirectoryContext(c.Request.Context(), "create", name)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
	_ = out

}

func setSyncSettingHTTP(c *gin.Context, kind, field string, local bool) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	raw, err := json.Marshal(arg[field])
	var normalized json.RawMessage
	if err == nil {
		normalized, err = model.SetSyncSettingContext(c.Request.Context(), kind, raw)
	}
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
	if local {
		ret.Data = map[string]interface{}{"local": normalized}
	}
}
func setSyncGenerateConflictDoc(c *gin.Context) {
	setSyncSettingHTTP(c, "syncConflict", "enabled", false)
}
func setSyncEnable(c *gin.Context)         { setSyncSettingHTTP(c, "syncEnabled", "enabled", false) }
func setSyncInterval(c *gin.Context)       { setSyncSettingHTTP(c, "syncInterval", "interval", false) }
func setSyncPerception(c *gin.Context)     { setSyncSettingHTTP(c, "syncPerception", "enabled", false) }
func setSyncMode(c *gin.Context)           { setSyncSettingHTTP(c, "syncMode", "mode", false) }
func setSyncProvider(c *gin.Context)       { setSyncSettingHTTP(c, "syncProvider", "provider", false) }
func setSyncProviderS3(c *gin.Context)     { setSyncSettingHTTP(c, "syncS3", "s3", false) }
func setSyncProviderWebDAV(c *gin.Context) { setSyncSettingHTTP(c, "syncWebDAV", "webdav", false) }
func setSyncProviderLocal(c *gin.Context)  { setSyncSettingHTTP(c, "syncLocal", "local", true) }

func setCloudSyncDir(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	name, ok := arg["name"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid name"
		return
	}
	owner := model.Conf
	out, e := model.ChangeCloudDirectoryContext(c.Request.Context(), "set", name)
	if e != nil {
		if errors.Is(e, cloudrepo.ErrName) {
			owner.NotifyCloudRepoError(c.Request.Context(), e, 5000)
			return
		}
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
	_ = out

}
