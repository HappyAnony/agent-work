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
	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
)

func extensionCopy(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(200, ret)
	// Match Gin's default multipart in-memory threshold. The model captures the
	// owner and parser before reading the body and owns multipart scratch cleanup.
	result, err := model.CopyExtensionRequest(c.Request.Context(), c.Request, 32<<20)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{"md": result.Markdown, "withMath": result.WithMath}
	ret.Msg = result.Message
}
