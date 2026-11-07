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
	"net/http"
	"wiki-kernel/internal/extensionprefs"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/conf"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func getSnippet(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	var input struct {
		Type    string `json:"type"`
		Enabled int    `json:"enabled"`
		Keyword string `json:"keyword"`
	}
	raw, e := json.Marshal(arg)
	if e == nil {
		e = json.Unmarshal(raw, &input)
	}
	if _, ok = arg["type"]; !ok {
		ret.Code = -1
		ret.Msg = "missing snippet type"
		return
	}
	if _, ok = arg["enabled"]; !ok {
		ret.Code = -1
		ret.Msg = "missing snippet enabled"
		return
	}
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
	isPublish := model.IsReadOnlyRole(model.GetGinContextRole(c))
	snippets, e := model.LoadSnippetsForRoleContext(c.Request.Context(), isPublish)
	if e != nil {
		ret.Code = -1
		ret.Msg = "load snippets failed: " + e.Error()
		return
	}
	ret.Data = map[string]interface{}{"snippets": extensionprefs.FilterSnippets(snippets, input.Type, input.Enabled, input.Keyword, isPublish)}
}
func setSnippet(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	raw, e := json.Marshal(arg["snippets"])
	var snippets []*conf.Snippet
	if e == nil {
		e = json.Unmarshal(raw, &snippets)
	}
	if e == nil {
		e = model.SetSnippetContext(c.Request.Context(), snippets)
	}
	if e != nil {
		ret.Code = -1
		ret.Msg = "set snippet failed: " + e.Error()
	}
}
func removeSnippet(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	id, ok := arg["id"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid snippet id"
		return
	}
	snippet, e := model.RemoveSnippetContext(c.Request.Context(), id)
	if e != nil {
		ret.Code = -1
		ret.Msg = "remove snippet failed: " + e.Error()
		return
	}
	ret.Data = snippet
}
