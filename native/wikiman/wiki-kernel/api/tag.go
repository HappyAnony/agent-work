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

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func getTag(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	sortParam := arg["sort"]
	var sortMode *int
	if nil != sortParam {
		value, valid := sortParam.(float64)
		if !valid || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value < -1<<31 || value > 1<<31-1 {
			ret.Code = -1
			ret.Msg = "invalid tag sort"
			return
		}
		mode := int(value)
		sortMode = &mode
	}

	// API `getTag` add an optional parameter `ignoreMaxListHint` https://github.com/siyuan-note/siyuan/issues/16000
	ignoreMaxListHint := false
	ignoreMaxListHintArg := arg["ignoreMaxListHint"]
	if nil != ignoreMaxListHintArg {
		var valid bool
		ignoreMaxListHint, valid = ignoreMaxListHintArg.(bool)
		if !valid {
			ret.Code = -1
			ret.Msg = "invalid tag hint"
			return
		}
	}

	app, valid := arg["app"].(string)
	if !valid {
		ret.Code = -1
		ret.Msg = "invalid tag app"
		return
	}
	data, err := model.BuildTagsContext(c.Request.Context(), sortMode, ignoreMaxListHint, app)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = data
}

func renameTag(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	oldLabel, oldOK := arg["oldLabel"].(string)
	newLabel, newOK := arg["newLabel"].(string)
	if !oldOK || !newOK {
		ret.Code = -1
		ret.Msg = "invalid tag"
		return
	}
	if err := model.RenameTagContext(c.Request.Context(), oldLabel, newLabel); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
}

func removeTag(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	label, valid := arg["label"].(string)
	if !valid {
		ret.Code = -1
		ret.Msg = "invalid tag"
		return
	}
	if err := model.RemoveTagContext(c.Request.Context(), label); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
}
