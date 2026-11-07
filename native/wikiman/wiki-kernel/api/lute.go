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
	"net/http"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func copyStdMarkdown(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id := arg["id"].(string)
	assetsDestSpace2Underscore := false
	if nil != arg["assetsDestSpace2Underscore"] {
		assetsDestSpace2Underscore = arg["assetsDestSpace2Underscore"].(bool)
	}

	fillCSSVar := false
	if nil != arg["fillCSSVar"] {
		fillCSSVar = arg["fillCSSVar"].(bool)
	}

	adjustHeadingLevel := false
	if nil != arg["adjustHeadingLevel"] {
		adjustHeadingLevel = arg["adjustHeadingLevel"].(bool)
	}

	imgTag := false
	if nil != arg["imgTag"] {
		imgTag = arg["imgTag"].(bool)
	}

	content, err := model.ExportStdMarkdownContext(c.Request.Context(), id, model.MarkdownContentOptions{AssetsDestSpace2Underscore: assetsDestSpace2Underscore, FillCSSVar: fillCSSVar, AdjustHeadingLevel: adjustHeadingLevel, ImgTag: imgTag})
	if err != nil && !errors.Is(err, model.ErrBlockNotFound) {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = content
}

func html2BlockDOM(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}
	output, err := model.HTMLPasteBlockDOM(c.Request.Context(), arg["dom"].(string))
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		ret.Data = "Failed to convert"
		return
	}
	ret.Data = output
}

func spinBlockDOM(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	dom := arg["dom"].(string)
	luteEngine, err := model.NewLuteContext(c.Request.Context())
	if err != nil {
		ret.Code, ret.Msg = -1, err.Error()
		return
	}

	dom = luteEngine.SpinBlockDOM(dom)
	ret.Data = map[string]interface{}{
		"dom": dom,
	}
}
