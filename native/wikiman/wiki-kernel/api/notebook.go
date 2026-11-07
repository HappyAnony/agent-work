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
	"github.com/siyuan-note/logging"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func getNotebookInfo(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	boxID := arg["notebook"].(string)
	if util.InvalidIDPattern(boxID, ret) {
		return
	}

	boxInfo, err := model.NotebookInfoContext(c.Request.Context(), boxID)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"boxInfo": boxInfo,
	}
}

func setNotebookIcon(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	boxID := arg["notebook"].(string)
	icon := arg["icon"].(string)
	if err := model.SetBoxIconContext(c.Request.Context(), boxID, icon); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

func changeSortNotebook(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	idsArg := arg["notebooks"].([]interface{})
	var ids []string
	for _, p := range idsArg {
		ids = append(ids, p.(string))
	}
	if err := model.ChangeBoxSortContext(c.Request.Context(), ids); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

func renameNotebook(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	notebook := arg["notebook"].(string)
	if util.InvalidIDPattern(notebook, ret) {
		return
	}

	name := arg["name"].(string)
	err := model.RenameBoxContext(c.Request.Context(), notebook, name)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}

	evt := util.NewCmdResult("renamenotebook", 0, util.PushModeBroadcast)
	evt.Data = map[string]interface{}{
		"box":  notebook,
		"name": name,
	}
	util.PushEvent(evt)
}

func removeNotebook(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	notebook := arg["notebook"].(string)
	if util.InvalidIDPattern(notebook, ret) {
		return
	}

	policy, policyErr := model.NotebookRemovePolicyContext(c.Request.Context(), notebook)
	if policyErr != nil {
		ret.Code = -1
		ret.Msg = policyErr.Error()
		return
	}
	if policy.CloseTimeout != 0 {
		ret.Code = -1
		ret.Msg = policy.Message
		ret.Data = map[string]interface{}{"closeTimeout": policy.CloseTimeout}
		return
	}

	err := model.RemoveBoxContext(c.Request.Context(), notebook)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	evt := util.NewCmdResult("unmount", 0, util.PushModeBroadcast)
	evt.Data = map[string]interface{}{
		"box": notebook,
	}
	evt.Callback = arg["callback"]
	util.PushEvent(evt)
}

func createNotebook(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	name := arg["name"].(string)
	id, err := model.CreateBoxContext(c.Request.Context(), name)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	existed, err := model.MountContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	box, err := model.NotebookContext(c.Request.Context(), id, true)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"notebook": box,
	}

	evt := util.NewCmdResult("createnotebook", 0, util.PushModeBroadcast)
	evt.Data = map[string]interface{}{
		"box":     box,
		"existed": existed,
	}
	util.PushEvent(evt)
}

func openNotebook(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	notebook := arg["notebook"].(string)
	if util.InvalidIDPattern(notebook, ret) {
		return
	}

	isUserGuide := model.IsUserGuide(notebook)
	policy, err := model.NotebookOpenPolicyContext(c.Request.Context(), notebook)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	if policy.CloseTimeout != 0 {
		ret.Code = -1
		ret.Msg = policy.Message
		ret.Data = map[string]interface{}{"closeTimeout": policy.CloseTimeout}
		return
	}
	clearProgress, err := model.NotebookOpenProgressContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	defer clearProgress()
	existed, err := model.MountContext(c.Request.Context(), notebook)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	box, err := model.NotebookContext(c.Request.Context(), notebook, true)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	evt := util.NewCmdResult("mount", 0, util.PushModeBroadcast)
	evt.Data = map[string]interface{}{
		"box":     box,
		"existed": existed,
	}
	evt.Callback = arg["callback"]
	util.PushEvent(evt)

	if isUserGuide {
		appArg := arg["app"]
		app := ""
		if nil != appArg {
			app = appArg.(string)
		}

		guideStartID := map[string]string{
			"20210808180117-czj9bvb": "20200812220555-lj3enxa",
			"20211226090932-5lcq56f": "20211226115423-d5z1joq",
			"20210808180117-6v0mkxr": "20200923234011-ieuun1p",
			"20240530133126-axarxgx": "20240530101000-4qitucx",
		}[notebook]
		if !model.ScheduleUserGuideOpenContext(c.Request.Context(), guideStartID, app) {
			logging.LogWarnf("user guide open task rejected")
		}
	}
}

func closeNotebook(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	notebook := arg["notebook"].(string)
	if util.InvalidIDPattern(notebook, ret) {
		return
	}
	if err := model.UnmountContext(c.Request.Context(), notebook); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}

func getNotebookConf(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	notebook := arg["notebook"].(string)
	if util.InvalidIDPattern(notebook, ret) {
		return
	}

	box, err := model.NotebookConfigurationContext(c.Request.Context(), notebook)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{"box": notebook, "name": box.Name, "conf": box}
}
func setNotebookConf(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	id := arg["notebook"].(string)
	if util.InvalidIDPattern(id, ret) {
		return
	}
	raw, err := gulu.JSON.MarshalJSON(arg["conf"])
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	value, err := model.SetNotebookConfigurationContext(c.Request.Context(), id, raw)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = value
}

func lsNotebooks(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	flashcard := false

	// 兼容旧版接口，不能直接使用 util.JsonArg()
	arg := map[string]interface{}{}
	if err := c.ShouldBindJSON(&arg); err == nil {
		if arg["flashcard"] != nil {
			flashcard = arg["flashcard"].(bool)
		}
	}

	var notebooks []*model.Box
	if flashcard {
		var err error
		notebooks, err = model.GetFlashcardNotebooksContext(c.Request.Context())
		if err != nil {
			ret.Code = -1
			ret.Msg = err.Error()
			return
		}
	} else {
		var err error
		notebooks, err = model.ListNotebooksContext(c.Request.Context())
		if err != nil {
			ret.Code = -1
			ret.Msg = err.Error()
			return
		}
	}

	ret.Data = map[string]interface{}{
		"notebooks": notebooks,
	}
}
