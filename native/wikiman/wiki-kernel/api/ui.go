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

func reloadTag(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if e := model.ReloadTagContext(c.Request.Context()); e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
}

func reloadFiletree(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if e := model.ReloadFiletreeContext(c.Request.Context()); e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
}

func reloadProtyle(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	if e := model.ReloadProtyleContext(c.Request.Context(), id); e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
}

func reloadAttributeView(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	if e := model.ReloadAttrViewContext(c.Request.Context(), id); e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
		return
	}
}

func reloadUI(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	util.ReloadUI()
}

func reloadIcon(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	if err := model.ReloadIconContext(c.Request.Context()); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
	}
}
