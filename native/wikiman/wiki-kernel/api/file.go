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
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"time"

	"github.com/88250/gulu"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gin-gonic/gin"
	"wiki-kernel/internal/indexrepair"
	"wiki-kernel/internal/workspacefile"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func getUniqueFilename(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	filePath := arg["path"].(string)
	out, err := model.UniqueHostFilenameContext(c.Request.Context(), filePath)
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	ret.Data = map[string]interface{}{"path": out}
}

func globalCopyFiles(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	var srcs []string
	for _, v := range arg["srcs"].([]interface{}) {
		srcs = append(srcs, v.(string))
	}
	if err := model.CopyWorkspaceFilesContext(c.Request.Context(), srcs, arg["destDir"].(string)); err != nil {
		ret.Code, ret.Msg = -1, err.Error()
	}
}
func copyFile(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	if err := c.Request.Context().Err(); err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	if err := model.CopyAssetToHostContext(c.Request.Context(), arg["src"].(string), arg["dest"].(string)); err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
	}
}

func getFile(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		ret.Code = -1
		c.JSON(http.StatusAccepted, ret)
		return
	}
	name := arg["path"].(string)
	out, err := model.WorkspaceFileContext(c.Request.Context(), workspacefile.Request{Action: "read", Path: name}, model.IsValidRole(model.GetGinContextRole(c), []model.Role{model.RoleAdministrator}))
	if err != nil {
		ret.Code = fileErrorCode(err)
		ret.Msg = err.Error()
		c.JSON(http.StatusAccepted, ret)
		return
	}
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = mimetype.Detect(out.Data).String()
	}
	c.Data(http.StatusOK, contentType, out.Data)
}
func readDir(c *gin.Context)    { applyWorkspaceFile(c, "list") }
func renameFile(c *gin.Context) { applyWorkspaceFile(c, "rename") }
func removeFile(c *gin.Context) { applyWorkspaceFile(c, "remove") }
func applyWorkspaceFile(c *gin.Context, action string) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	q := workspacefile.Request{Action: action, Path: arg["path"].(string)}
	if action == "rename" {
		q.NewPath = arg["newPath"].(string)
	}
	out, err := model.WorkspaceFileContext(c.Request.Context(), q, true)
	if err != nil {
		ret.Code = fileErrorCode(err)
		ret.Msg = err.Error()
		return
	}
	if action == "list" {
		ret.Data = out.Entries
	}
}
func putFile(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	if err := model.PutWorkspaceFileRequest(c.Request.Context(), c.Request); err != nil {
		ret.Code = fileErrorCode(err)
		ret.Msg = err.Error()
	}
}
func fileErrorCode(err error) int {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, fs.ErrExist):
		return http.StatusConflict
	case errors.Is(err, fs.ErrPermission) || errors.Is(err, indexrepair.ErrSourcePath) || errors.Is(err, model.ErrSettingReadOnly):
		return http.StatusForbidden
	case errors.Is(err, workspacefile.ErrDirectory) || errors.Is(err, workspacefile.ErrNotDirectory):
		return http.StatusMethodNotAllowed
	}
	return http.StatusInternalServerError
}

func millisecond2Time(t int64) time.Time {
	sec := t / 1000
	msec := t % 1000
	return time.Unix(sec, msec*int64(time.Millisecond))
}
