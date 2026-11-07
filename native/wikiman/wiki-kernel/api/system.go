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
	"net/http"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"github.com/siyuan-note/logging"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func vacuumDataIndex(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if err := model.VacuumDataIndexContext(c.Request.Context()); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

func rebuildDataIndex(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if err := model.FullReindexContext(c.Request.Context()); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

func addMicrosoftDefenderExclusion(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if !gulu.OS.IsWindows() {
		return
	}

	err := model.AddMicrosoftDefenderExclusionContext(c.Request.Context())
	if nil != err {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

func ignoreAddMicrosoftDefenderExclusion(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if !gulu.OS.IsWindows() {
		return
	}

	if _, err := model.SetDeviceConfigurationContext(c.Request.Context(), "defenderExcluded", []byte(`{}`)); err != nil {
		ret.Code, ret.Msg = -1, err.Error()
	}
}

func getWorkspaceInfo(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	out, err := model.WorkspaceInfoContext(c.Request.Context())
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	ret.Data = out
}

func getNetwork(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	maskedConf, err := model.GetMaskedConfContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"proxy": maskedConf.System.NetworkProxy,
	}
}

func getChangelog(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	out, err := model.ChangelogContext(c.Request.Context())
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	ret.Data = out
}

func getEmojiConf(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	data, err := model.EmojiConfigurationContext(c.Request.Context())
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	ret.Data = data
}

func exportLog(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	zipPath, err := model.ExportSystemLogContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"zip": zipPath,
	}
}

func exportConf(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	out, err := model.ExportSettingPackageContext(c.Request.Context(), "conf")
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = out
}

func importConf(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	if err := model.ImportConfigurationRequest(c.Request.Context(), c.Request); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

func getConf(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	role := model.GetGinContextRole(c)
	isPublish := model.IsReadOnlyRole(role)
	restricted := !model.IsValidRole(role, []model.Role{model.RoleAdministrator})
	maskedConf, err := model.ConfigurationPresentationContext(c.Request.Context(), restricted, isPublish)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	confData := map[string]interface{}{
		"conf":      maskedConf,
		"start":     !util.IsUILoaded.Load(),
		"isPublish": isPublish,
	}

	// M6: project single-mode runtime fields into data.conf.system.*
	// when kernel runs with --single. In multi-mode, the keys are absent
	// (not present-as-false), per Architecture §02 §4.1 and api-surface.md §3.1.
	// util.SingleNotebookMode is the source of truth populated directly from
	// --single / WIKI_SINGLE_NOTEBOOK during process boot. Conf carries a
	// runtime mirror populated later during notebook bootstrap, so using it as
	// the gate can briefly (or after a workspace switch) advertise a single
	// kernel as a normal multi-notebook kernel to the iframe.
	if util.SingleNotebookMode {
		injectSingleModeFields(confData)
	}

	ret.Data = confData
}

// injectSingleModeFields mutates `data["conf"]` so that the JSON-serialized
// response payload includes the five single-notebook-mode fields under
// data.conf.system.* — per api-surface.md §3.1 and Architecture §02 §4.1.
//
// Why this is a "JSON projection" (path b) rather than a struct embedding
// (path a): the 5 source-of-truth fields live on model.AppConf with
// `json:"-"` tags (see Engineering/be/02-single-conf-fields.md §EA-M2-2)
// so that AppConf.Save() does not persist them to conf.json. Putting them
// on kernel/conf.System would require importing model.UnmountedBoxInfo /
// model.IgnoredDocInfo into the conf package, which would create a
// circular import (model already imports conf). Marshal-unmarshal to a
// generic map is the cleanest path that avoids both issues.
//
// On any marshal/unmarshal error the function logs and returns without
// modifying data — the FE then receives a payload with system.single
// absent and enters fallback B ("kernel not started with --single").
// Fallback is preferable to crashing the whole getConf call.
func resolveSingleModeIDs() (boxID, rootDocID string) {
	// Prefer the process-level state set by single-mode bootstrap. The config
	// mirror is intentionally non-persistent and may lag after a workspace
	// switch, while these util values are the values used by the guard itself.
	boxID = util.SingleNotebookBoxID
	rootDocID = util.SingleNotebookRootDocID
	// This projection must remain read-only, including during startup or a
	// transition. Bootstrap belongs to the lifecycle owner after DB readiness;
	// invoking it here can enqueue source mutations against unopened indexes.
	if boxID == "" {
		boxID = model.Conf.SingleNotebookBoxID
	}
	if rootDocID == "" {
		rootDocID = model.Conf.SingleNotebookRootDocID
	}
	return
}

func injectSingleModeFields(data map[string]interface{}) {
	confBytes, err := gulu.JSON.MarshalJSON(data["conf"])
	if err != nil {
		logging.LogErrorf("project single-mode getConf payload: marshal conf failed: %s", err)
		return
	}
	var confMap map[string]interface{}
	if err := gulu.JSON.UnmarshalJSON(confBytes, &confMap); err != nil {
		logging.LogErrorf("project single-mode getConf payload: unmarshal conf failed: %s", err)
		return
	}

	systemAny, ok := confMap["system"]
	if !ok {
		// AppConf.System should always exist; if absent, build an empty map
		// so the FE can still detect single mode via the `single` flag.
		systemAny = map[string]interface{}{}
	}
	systemMap, ok := systemAny.(map[string]interface{})
	if !ok {
		logging.LogErrorf("project single-mode getConf payload: conf.system is not a JSON object")
		return
	}

	boxID, rootDocID := resolveSingleModeIDs()
	logging.LogInfof("event=single_mode_get_conf boxID=%s rootDocID=%s", boxID, rootDocID)

	systemMap["single"] = true
	systemMap["singleNotebookBoxID"] = boxID
	systemMap["singleNotebookRootDocID"] = rootDocID

	// Per Architecture §02 §4.1: lists may be empty but keys must be present.
	// Emit [] (not null) when M3 left the slice as nil.
	if model.Conf.UnmountedBoxes == nil {
		systemMap["unmountedBoxes"] = []model.UnmountedBoxInfo{}
	} else {
		systemMap["unmountedBoxes"] = model.Conf.UnmountedBoxes
	}
	if model.Conf.IgnoredDocs == nil {
		systemMap["ignoredDocs"] = []model.IgnoredDocInfo{}
	} else {
		systemMap["ignoredDocs"] = model.Conf.IgnoredDocs
	}

	confMap["system"] = systemMap
	data["conf"] = confMap
}

func setUILayout(c *gin.Context) { applyDeviceConfiguration(c, "uiLayout") }

func setAPIToken(c *gin.Context) { applyDeviceConfiguration(c, "apiToken") }

func setAccessAuthCode(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	raw, err := json.Marshal(arg)
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	value, err := model.SetDeviceConfigurationContext(c.Request.Context(), "accessAuthCode", raw)
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	var aac string
	if err = json.Unmarshal(value, &aac); err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}

	session := util.GetSession(c)
	workspaceSession := util.GetWorkspaceSession(session)
	workspaceSession.AccessAuthCode = aac
	session.Save(c)
	if !model.ScheduleLegacyUIReloadContext(c.Request.Context()) {
		logging.LogWarnf("delayed UI reload task rejected")
	}
	return
}

func setFollowSystemLockScreen(c *gin.Context) { applyDeviceConfiguration(c, "lockScreenMode") }

func getSysFonts(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	ret.Data = util.LoadSysFonts()
}

func version(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	ret.Data = util.Ver
}

func currentTime(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	ret.Data = util.CurrentTimeMillis()
}

func bootProgress(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	progress, details := util.GetBootProgressDetails()
	ret.Data = map[string]interface{}{"progress": progress, "details": details}
}

func setAppearanceMode(c *gin.Context) { applyDeviceConfiguration(c, "appearanceMode") }

func setNetworkServe(c *gin.Context) { applyDeviceConfiguration(c, "networkServe") }

func setAutoLaunch(c *gin.Context) { applyDeviceConfiguration(c, "autoLaunch") }

func setDownloadInstallPkg(c *gin.Context) { applyDeviceConfiguration(c, "downloadInstallPkg") }

func setNetworkProxy(c *gin.Context) { applyDeviceConfiguration(c, "networkProxy") }

func addUIProcess(c *gin.Context) {
	pid := c.Query("pid")
	util.UIProcessIDs.Store(pid, true)
}

func exit(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	forceArg := arg["force"]
	var force bool
	if nil != forceArg {
		force = forceArg.(bool)
	}

	execInstallPkgArg := arg["execInstallPkg"] // 0：默认检查新版本，1：不执行新版本安装，2：执行新版本安装
	execInstallPkg := 0
	if nil != execInstallPkgArg {
		execInstallPkg = int(execInstallPkgArg.(float64))
	}

	exitCode := model.CloseContext(context.WithoutCancel(c.Request.Context()), force, true, execInstallPkg)
	ret.Code = exitCode
	switch exitCode {
	case 0:
	case 1: // 同步执行失败
		ret.Msg = model.Conf.Language(96) + "<div class=\"fn__space\"></div><button class=\"b3-button b3-button--white\">" + model.Conf.Language(97) + "</button>"
		ret.Data = map[string]interface{}{"closeTimeout": 0}
	case 2: // 提示新安装包
		ret.Msg = model.Conf.Language(61)
		ret.Data = map[string]interface{}{"closeTimeout": 0}
	}
}

func applyDeviceConfiguration(c *gin.Context, kind string) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	raw, err := json.Marshal(arg)
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	value, err := model.SetDeviceConfigurationContext(c.Request.Context(), kind, raw)
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	if kind == "appearanceMode" {
		ret.Data = map[string]any{"appearance": json.RawMessage(value)}
	}
}
