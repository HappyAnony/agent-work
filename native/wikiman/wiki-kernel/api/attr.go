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

func getBookmarkLabels(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	data, err := model.BookmarkLabelsContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = data
}

func batchGetBlockAttrs(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	ids := arg["ids"].([]interface{})
	var idList []string
	for _, id := range ids {
		idList = append(idList, id.(string))
	}

	out, err := model.ReadBlockAttributesContext(c.Request.Context(), idList)
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	ret.Data = out
}

func getBlockAttrs(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	if util.InvalidIDPattern(id, ret) {
		return
	}

	out, err := model.ReadBlockAttributesContext(c.Request.Context(), []string{id})
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}
	attrs := out[id]
	if attrs == nil {
		attrs = map[string]string{}
	}
	ret.Data = attrs
}

func setBlockAttrs(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	if util.InvalidIDPattern(id, ret) {
		return
	}

	attrs := arg["attrs"].(map[string]interface{})

	nameValues := map[string]string{}
	for name, value := range attrs {
		if nil == value { // API `setBlockAttrs` 中如果存在属性值设置为 `null` 时移除该属性 https://github.com/siyuan-note/siyuan/issues/5577
			nameValues[name] = ""
		} else {
			nameValues[name] = value.(string)
		}
	}
	err := model.SetBlockAttrsContext(c.Request.Context(), id, nameValues)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
}

func batchSetBlockAttrs(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	blockAttrsArg := arg["blockAttrs"].([]interface{})
	var blockAttrs []map[string]interface{}
	for _, blockAttrArg := range blockAttrsArg {
		blockAttr := blockAttrArg.(map[string]interface{})
		id := blockAttr["id"].(string)
		if util.InvalidIDPattern(id, ret) {
			return
		}

		attrs := blockAttr["attrs"].(map[string]interface{})
		nameValues := map[string]string{}
		for name, value := range attrs {
			if nil == value {
				nameValues[name] = ""
			} else {
				nameValues[name] = value.(string)
			}
		}

		blockAttrs = append(blockAttrs, map[string]interface{}{
			"id":    id,
			"attrs": nameValues,
		})
	}

	err := model.BatchSetBlockAttrsContext(c.Request.Context(), blockAttrs)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
}

func resetBlockAttrs(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	attrs := arg["attrs"].(map[string]interface{})
	nameValues := map[string]string{}
	for name, value := range attrs {
		nameValues[name] = value.(string)
	}
	err := model.ResetBlockAttrsContext(c.Request.Context(), id, nameValues)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
}
