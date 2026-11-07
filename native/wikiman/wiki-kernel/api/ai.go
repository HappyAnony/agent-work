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

func chatGPT(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	msg, ok := arg["msg"].(string)
	if !ok {
		ret.Code = -1
		ret.Msg = "invalid AI arguments"
		return
	}
	text, e := model.ChatGPTContext(c.Request.Context(), msg)
	ret.Data = text
	if c.Request.Context().Err() != nil && e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
	}
}

func chatGPTWithAction(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	idsArg, valid := arg["ids"].([]interface{})
	if !valid {
		ret.Code = -1
		ret.Msg = "invalid AI arguments"
		return
	}
	var ids []string
	for _, id := range idsArg {
		v, valid := id.(string)
		if !valid {
			ret.Code = -1
			ret.Msg = "invalid AI arguments"
			return
		}
		ids = append(ids, v)
	}
	action, valid := arg["action"].(string)
	if !valid {
		ret.Code = -1
		ret.Msg = "invalid AI arguments"
		return
	}
	text, e := model.ChatGPTWithActionContext(c.Request.Context(), ids, action)
	ret.Data = text
	if c.Request.Context().Err() != nil && e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
	}
}
