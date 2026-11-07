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
	"wiki-kernel/conf"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func resetGraph(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	graph := conf.NewGlobalGraph()
	raw, _ := gulu.JSON.MarshalJSON(graph)
	if _, err := model.SetGraphSettingContext(c.Request.Context(), "graphGlobal", raw); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"conf": graph,
	}
}

func resetLocalGraph(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	graph := conf.NewLocalGraph()
	raw, _ := gulu.JSON.MarshalJSON(graph)
	if _, err := model.SetGraphSettingContext(c.Request.Context(), "graphLocal", raw); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"conf": graph,
	}
}

func getGraph(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	reqId := arg["reqId"]
	ret.Data = map[string]interface{}{"reqId": reqId}

	query := arg["k"].(string)
	graphConf, err := gulu.JSON.MarshalJSON(arg["conf"])
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	global := conf.NewGlobalGraph()
	if err = gulu.JSON.UnmarshalJSON(graphConf, global); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	result, err := model.GraphContext(c.Request.Context(), "", query, "graphGlobal", graphConf)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	boxID, nodes, links := result.Box, result.Nodes, result.Links
	ret.Data = map[string]interface{}{
		"nodes": nodes,
		"links": links,
		"conf":  global,
		"box":   boxID,
		"reqId": arg["reqId"],
	}
}

func getLocalGraph(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	reqId := arg["reqId"]
	ret.Data = map[string]interface{}{"reqId": reqId}
	if nil == arg["id"] {
		return
	}

	keyword := arg["k"].(string)
	id := arg["id"].(string)

	graphConf, err := gulu.JSON.MarshalJSON(arg["conf"])
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	local := conf.NewLocalGraph()
	if err = gulu.JSON.UnmarshalJSON(graphConf, local); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	result, err := model.GraphContext(c.Request.Context(), id, keyword, "graphLocal", graphConf)
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	boxID, nodes, links := result.Box, result.Nodes, result.Links
	ret.Data = map[string]interface{}{
		"id":    id,
		"box":   boxID,
		"nodes": nodes,
		"links": links,
		"conf":  local,
		"reqId": arg["reqId"],
	}
}
