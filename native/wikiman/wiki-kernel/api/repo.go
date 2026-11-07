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
	"fmt"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/88250/gulu"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gin-gonic/gin"
	"wiki-kernel/internal/cloudrepo"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func setRepoIndexRetentionDays(c *gin.Context) {
	applyConfigurationSetting(c, "repoIndexRetention", false)
}

func setRetentionIndexesDaily(c *gin.Context) {
	applyConfigurationSetting(c, "repoRetentionDaily", false)
}

func getRepoFile(c *gin.Context) {
	// Add internal kernel API `/api/repo/getRepoFile` https://github.com/siyuan-note/siyuan/issues/10101

	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	data, p, err := model.GetRepoFileContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(p))
	if "" == contentType {
		if m := mimetype.Detect(data); nil != m {
			contentType = m.String()
		}
	}
	if "" == contentType {
		contentType = "application/octet-stream"
	}
	c.Data(http.StatusOK, contentType, data)
}

func openRepoSnapshotDoc(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	title, content, displayInText, updated, err := model.OpenRepoSnapshotDocContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"title":         title,
		"content":       content,
		"displayInText": displayInText,
		"updated":       updated,
	}
}

func diffRepoSnapshots(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	left := arg["left"].(string)
	right := arg["right"].(string)
	diff, err := model.DiffRepoSnapshotsContext(c.Request.Context(), left, right)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"addsLeft":     diff.AddsLeft,
		"updatesLeft":  diff.UpdatesLeft,
		"updatesRight": diff.UpdatesRight,
		"removesRight": diff.RemovesRight,
		"left":         diff.LeftIndex,
		"right":        diff.RightIndex,
	}
}

func getCloudSpace(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	owner, workspace, data := model.Conf, util.WorkspaceDir, util.DataDir
	page := 1
	out, e := model.QueryCloudRepoContext(c.Request.Context(), "space", page)
	if e != nil {
		ret.Code = 1
		ret.Msg = e.Error()
		if owner == model.Conf && workspace == util.WorkspaceDir && data == util.DataDir {
			owner.NotifyCloudRepoError(c.Request.Context(), e)
		}
		return
	}
	ret.Data = out.Data

}

func checkoutRepo(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id, valid := arg["id"].(string)
	if !valid || !cloudrepo.ValidSnapshotID(id) {
		ret.Code = -1
		ret.Msg = "invalid snapshot id"
		return
	}
	if e := model.EnqueueRepoCheckoutContext(c.Request.Context(), id); e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
	}
}

func downloadCloudSnapshot(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	id, ok := arg["id"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid id"
		return
	}
	tag, ok := arg["tag"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid tag"
		return
	}
	_, e := model.ChangeSnapshotContext(c.Request.Context(), "download", id, tag)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}

}

func uploadCloudSnapshot(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	id, ok := arg["id"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid id"
		return
	}
	tag, ok := arg["tag"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid tag"
		return
	}
	_, e := model.ChangeSnapshotContext(c.Request.Context(), "upload", id, tag)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}

}

func getRepoSnapshots(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	page := arg["page"].(float64)
	snapshots, pageCount, totalCount, err := model.GetRepoSnapshotsContext(c.Request.Context(), int(page))
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"snapshots":  snapshots,
		"pageCount":  pageCount,
		"totalCount": totalCount,
	}
}

func getCloudRepoSnapshots(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	page := 1
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	v, ok := arg["page"].(float64)
	if !ok || v > 1000000 || v != float64(int(v)) {
		ret.Code = -1
		ret.Msg = "invalid page"
		return
	}
	page = int(v)
	out, e := model.QueryCloudRepoContext(c.Request.Context(), "logs", page)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
	ret.Data = out.Data

}

func getCloudRepoTagSnapshots(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	page := 1
	out, e := model.QueryCloudRepoContext(c.Request.Context(), "tags", page)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
	ret.Data = out.Data

}

func removeCloudRepoTagSnapshot(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	tag, ok := arg["tag"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid tag"
		return
	}
	_, e := model.ChangeSnapshotContext(c.Request.Context(), "cloud-untag", "", tag)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}

}

func getRepoTagSnapshots(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	snapshots, err := model.GetTagSnapshotsContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = map[string]interface{}{
		"snapshots": snapshots,
	}
}

func removeRepoTagSnapshot(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	tag, ok := arg["tag"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid tag"
		return
	}
	_, e := model.ChangeSnapshotContext(c.Request.Context(), "untag", "", tag)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}

}

func createSnapshot(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	memo, valid := arg["memo"].(string)
	if !valid {
		ret.Code = -1
		ret.Msg = "invalid memo"
		return
	}
	if err := model.IndexRepoContext(c.Request.Context(), memo); err != nil {
		ret.Code = -1
		ret.Msg = fmt.Sprintf(model.Conf.Language(140), err)
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
}

func tagSnapshot(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	owner := model.Conf
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	id, ok := arg["id"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid id"
		return
	}
	name, ok := arg["name"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid name"
		return
	}
	_, e := model.ChangeSnapshotContext(c.Request.Context(), "tag", id, name)
	if e != nil {
		ret.Code = -1
		ret.Msg = fmt.Sprintf(owner.Language(140), e.Error())
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
	_ = owner

}

func importRepoKey(c *gin.Context) {
	owner := model.Conf
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	base64Key, valid := arg["key"].(string)
	if !valid {
		ret.Code = -1
		ret.Msg = "invalid key"
		return
	}
	result, err := model.ChangeRepoKeyContext(c.Request.Context(), "import", base64Key)
	if err != nil {
		ret.Code = -1
		ret.Msg = fmt.Sprintf(owner.Language(137), err)
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}

	ret.Data = map[string]interface{}{
		"key": result.Normalized,
	}
}

func initRepoKeyFromPassphrase(c *gin.Context) {
	owner := model.Conf
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	pass, valid := arg["pass"].(string)
	if !valid {
		ret.Code = -1
		ret.Msg = "invalid pass"
		return
	}
	result, err := model.ChangeRepoKeyContext(c.Request.Context(), "passphrase", pass)
	if err != nil {
		ret.Code = -1
		ret.Msg = fmt.Sprintf(owner.Language(137), err)
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}

	ret.Data = map[string]interface{}{
		"key": result.Key,
	}
}

func initRepoKey(c *gin.Context) {
	owner := model.Conf
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	result, err := model.ChangeRepoKeyContext(c.Request.Context(), "random", "")
	if err != nil {
		ret.Code = -1
		ret.Msg = fmt.Sprintf(owner.Language(137), err)
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}

	ret.Data = map[string]interface{}{
		"key": result.Key,
	}
}

func resetRepo(c *gin.Context) {
	owner := model.Conf
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	_, err := model.ChangeRepoKeyContext(c.Request.Context(), "reset", "")
	if err != nil {
		ret.Code = -1
		ret.Msg = fmt.Sprintf(owner.Language(146), err.Error())
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
}

func purgeRepo(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	owner := model.Conf
	_, e := model.ChangeSnapshotContext(c.Request.Context(), "purge-local", "", "")
	if e != nil {
		ret.Code = -1
		ret.Msg = fmt.Sprintf(owner.Language(201), e.Error())
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
	_ = owner

}

func purgeCloudRepo(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	owner := model.Conf
	_, e := model.ChangeSnapshotContext(c.Request.Context(), "purge-cloud", "", "")
	if e != nil {
		ret.Code = -1
		ret.Msg = fmt.Sprintf(owner.Language(201), e.Error())
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
	_ = owner

}
