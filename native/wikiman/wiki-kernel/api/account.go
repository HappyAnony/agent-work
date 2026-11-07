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
	"math"
	"net/http"
	"wiki-kernel/internal/cloudaccount"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func startFreeTrial(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	err := model.StartFreeTrialContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
}

func useActivationcode(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	code, valid := arg["data"].(string)
	if !valid {
		ret.Code = -1
		ret.Msg = cloudaccount.ErrResponse.Error()
		return
	}
	err := model.UseActivationcodeContext(c.Request.Context(), code)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
}

func checkActivationcode(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	code, valid := arg["data"].(string)
	if !valid {
		ret.Code = 1
		ret.Msg = cloudaccount.ErrResponse.Error()
		return
	}
	ret.Code, ret.Msg = model.CheckActivationcodeContext(c.Request.Context(), code)
}

func deactivateUser(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	err := model.DeactivateUserContext(c.Request.Context())
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
}

func login(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	arg, ok := util.JsonArg(c, ret)
	if !ok {
		c.JSON(http.StatusOK, ret)
		return
	}

	name, nok := arg["userName"].(string)
	password, pok := arg["userPassword"].(string)
	captcha, cok := arg["captcha"].(string)
	region, rok := arg["cloudRegion"].(float64)
	if !nok || !pok || !cok || !rok || math.IsNaN(region) || math.IsInf(region, 0) || region != math.Trunc(region) || region < -2147483648 || region > 2147483647 {
		ret.Code = -1
		ret.Msg = cloudaccount.ErrResponse.Error()
		c.JSON(http.StatusOK, ret)
		return
	}
	v, e := model.LoginContext(c.Request.Context(), name, password, captcha, int(region))
	if e != nil {
		cloudAccountError(ret, e)
	} else {
		ret.Code = v.Code
		ret.Msg = v.Msg
		ret.Data = v.Data
	}
	c.JSON(http.StatusOK, ret)
}

func cloudAccountError(ret *gulu.Result, e error) {
	ret.Code = -1
	ret.Msg = e.Error()
	if model.Conf != nil && (errors.Is(e, cloudaccount.ErrNetwork) || errors.Is(e, cloudaccount.ErrResponse)) {
		ret.Msg = model.Conf.Language(18)
	}
}
