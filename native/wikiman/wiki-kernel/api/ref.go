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
	"strconv"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func refreshBacklink(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	id, ok := arg["id"].(string)
	if !ok || id == "" {
		ret.Code = -1
		ret.Msg = "invalid block id"
		return
	}
	if e := model.RefreshBacklinkContext(c.Request.Context(), id); e != nil {
		ret.Code = -1
		ret.Msg = e.Error()
	}
}

func getBackmentionDoc(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	defID := arg["defID"].(string)
	refTreeID := arg["refTreeID"].(string)
	keyword := arg["keyword"].(string)
	containChildren := false
	_, specifiedChildren := arg["containChildren"]
	if val, ok := arg["containChildren"]; ok {
		containChildren = val.(bool)
	}
	highlight := true
	if val, ok := arg["highlight"]; ok {
		highlight = val.(bool)
	}
	value, err := model.BacklinksContext(c.Request.Context(), blockview.BacklinkRequest{Mode: "mention-document", ID: defID, RefTreeID: refTreeID, Keyword: keyword, Children: containChildren, DefaultChildren: !specifiedChildren, Highlight: highlight})
	backlinks, keywords := value.Documents, value.Keywords
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"backmentions": backlinks,
		"keywords":     keywords,
	}
}

func getBacklinkDoc(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	defID := arg["defID"].(string)
	refTreeID := arg["refTreeID"].(string)
	keyword := arg["keyword"].(string)
	containChildren := false
	_, specifiedChildren := arg["containChildren"]
	if val, ok := arg["containChildren"]; ok {
		containChildren = val.(bool)
	}
	highlight := true
	if val, ok := arg["highlight"]; ok {
		highlight = val.(bool)
	}
	value, err := model.BacklinksContext(c.Request.Context(), blockview.BacklinkRequest{Mode: "document", ID: defID, RefTreeID: refTreeID, Keyword: keyword, Children: containChildren, DefaultChildren: !specifiedChildren, Highlight: highlight})
	backlinks, keywords := value.Documents, value.Keywords
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"backlinks": backlinks,
		"keywords":  keywords,
	}
}

func getBacklink2(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	if nil == arg["id"] {
		return
	}

	id := arg["id"].(string)
	keyword := arg["k"].(string)
	mentionKeyword := arg["mk"].(string)
	sortArg := arg["sort"]
	sort := util.SortModeUpdatedDESC
	if nil != sortArg {
		sort, _ = strconv.Atoi(sortArg.(string))
	}
	mentionSortArg := arg["mSort"]
	mentionSort := util.SortModeUpdatedDESC
	if nil != mentionSortArg {
		mentionSort, _ = strconv.Atoi(mentionSortArg.(string))
	}
	containChildren := false
	_, specifiedChildren := arg["containChildren"]
	if val, ok := arg["containChildren"]; ok {
		containChildren = val.(bool)
	}
	value, err := model.BacklinksContext(c.Request.Context(), blockview.BacklinkRequest{Mode: "links2", ID: id, Keyword: keyword, MentionKeyword: mentionKeyword, SortMode: sort, MentionSortMode: mentionSort, Children: containChildren, DefaultChildren: !specifiedChildren})
	boxID, backlinks, backmentions, linkRefsCount, mentionsCount := value.Box, value.Links, value.Mentions, value.ReferenceCount, value.MentionCount
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"backlinks":     backlinks,
		"linkRefsCount": linkRefsCount,
		"backmentions":  backmentions,
		"mentionsCount": mentionsCount,
		"k":             keyword,
		"mk":            mentionKeyword,
		"box":           boxID,
	}
}

func getBacklink(c *gin.Context) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	if nil == arg["id"] {
		return
	}

	id := arg["id"].(string)
	keyword := arg["k"].(string)
	mentionKeyword := arg["mk"].(string)
	beforeLen := 12
	if nil != arg["beforeLen"] {
		beforeLen = int(arg["beforeLen"].(float64))
	}
	containChildren := false
	_, specifiedChildren := arg["containChildren"]
	if val, ok := arg["containChildren"]; ok {
		containChildren = val.(bool)
	}
	value, err := model.BacklinksContext(c.Request.Context(), blockview.BacklinkRequest{Mode: "links", ID: id, Keyword: keyword, MentionKeyword: mentionKeyword, BeforeLen: beforeLen, Children: containChildren, DefaultChildren: !specifiedChildren})
	boxID, backlinks, backmentions, linkRefsCount, mentionsCount := value.Box, value.Links, value.Mentions, value.ReferenceCount, value.MentionCount
	if err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	ret.Data = map[string]interface{}{
		"backlinks":     backlinks,
		"linkRefsCount": linkRefsCount,
		"backmentions":  backmentions,
		"mentionsCount": mentionsCount,
		"k":             keyword,
		"mk":            mentionKeyword,
		"box":           boxID,
	}
	util.RandomSleep(200, 500)
}
