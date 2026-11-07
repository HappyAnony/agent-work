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
	"math"
	"net/http"
	"wiki-kernel/internal/cloudaccount"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func removeShorthands(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	idsArg, valid := arg["ids"].([]interface{})
	if !valid {
		ret.Code = 1
		ret.Msg = cloudaccount.ErrResponse.Error()
		return
	}
	var ids []string
	for _, value := range idsArg {
		id, valid := value.(string)
		if !valid {
			ret.Code = 1
			ret.Msg = cloudaccount.ErrResponse.Error()
			return
		}
		ids = append(ids, id)
	}
	err := model.RemoveCloudShorthandsContext(c.Request.Context(), ids)
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		return
	}
}

func getShorthand(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id, valid := arg["id"].(string)
	if !valid {
		ret.Code = 1
		ret.Msg = cloudaccount.ErrResponse.Error()
		return
	}
	data, err := model.GetCloudShorthandContext(c.Request.Context(), id)
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		return
	}
	ret.Data = data
}

func getShorthands(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	page, valid := arg["page"].(float64)
	if !valid || math.IsNaN(page) || math.IsInf(page, 0) || page != math.Trunc(page) || page < -2147483648 || page > 2147483647 {
		ret.Code = 1
		ret.Msg = cloudaccount.ErrResponse.Error()
		return
	}
	data, err := model.GetCloudShorthandsContext(c.Request.Context(), int(page))
	if err != nil {
		ret.Code = 1
		ret.Msg = err.Error()
		return
	}
	ret.Data = data
}
