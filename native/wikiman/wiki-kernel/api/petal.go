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

func loadPetals(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	frontend, ok := arg["frontend"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid plugin frontend"
		return
	}
	data, e := model.LoadPetalsContext(c.Request.Context(), frontend, model.IsReadOnlyRole(model.GetGinContextRole(c)))
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
	ret.Data = data
}
func setPetalEnabled(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	name, n := arg["packageName"].(string)
	enabled, b := arg["enabled"].(bool)
	frontend, f := arg["frontend"].(string)
	if !n || !b || !f {
		ret.Code = -1
		ret.Msg = "invalid plugin fields"
		return
	}
	app := ""
	if arg["app"] != nil {
		var ok bool
		app, ok = arg["app"].(string)
		if !ok {
			ret.Code = -1
			ret.Msg = "invalid plugin app"
			return
		}
	}
	data, e := model.SetPetalEnabledContext(c.Request.Context(), name, enabled, frontend, app, true)
	if e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
	ret.Data = data
}
