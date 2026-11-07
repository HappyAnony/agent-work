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
	"encoding/json"
	"errors"
	"net/http"
	"wiki-kernel/internal/pageprefs"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func storageHTTP(c *gin.Context, action, keyField, valueField, event string, read, bind bool) {
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)
	arg := map[string]interface{}{}
	if bind {
		var ok bool
		arg, ok = util.JsonArg(c, ret)
		if !ok {
			return
		}
	} else if action == "recent.get" {
		_ = c.ShouldBindJSON(&arg)
	}
	q := pageprefs.Request{Action: action, SortBy: "viewedAt"}
	fail := func(msg string) { ret.Code = -1; ret.Msg = msg }
	if action == "recent.close" && arg["rootID"] == nil {
		return
	}
	if keyField != "" {
		key, ok := arg[keyField].(string)
		if !ok {
			fail("invalid storage key")
			return
		}
		q.Key = key
	}
	if action == "recent.get" && arg["sortBy"] != nil {
		value, ok := arg["sortBy"].(string)
		if !ok {
			fail("invalid recent sort")
			return
		}
		q.SortBy = value
	}
	var err error
	if valueField != "" {
		v, ok := arg[valueField]
		if !ok {
			fail("missing storage value")
			return
		}
		q.Value, err = json.Marshal(v)
		if err != nil {
			fail(err.Error())
			return
		}
	}
	if action == "local.remove" {
		keys, ok := arg["keys"].([]interface{})
		if !ok {
			fail("invalid storage keys")
			return
		}
		for _, key := range keys {
			k, ok := key.(string)
			if !ok {
				fail("invalid storage key")
				return
			}
			q.Keys = append(q.Keys, k)
		}
	}
	var after func()
	if event != "" {
		app, ok := arg["app"].(string)
		if !ok {
			fail("invalid storage app")
			return
		}
		evt := util.NewCmdResult(event, 0, util.PushModeBroadcastMainExcludeSelfApp)
		evt.AppId = app
		switch action {
		case "local.remove":
			evt.Data = map[string]interface{}{"keys": q.Keys}
		case "local.setValue":
			evt.Data = map[string]interface{}{"key": q.Key, "val": arg["val"]}
		case "local.set":
			evt.Data = arg["val"]
		default:
			err = errors.New("invalid storage event")
		}
		if err != nil {
			fail(err.Error())
			return
		}
		after = func() { util.PushEvent(evt) }
	}
	value, err := model.StorageContextWithAfter(c.Request.Context(), q, after)
	if err != nil {
		fail(err.Error())
		return
	}
	if read {
		ret.Data = value
	}
}
func getRecentDocs(c *gin.Context)   { storageHTTP(c, "recent.get", "", "", "", true, false) }
func removeCriterion(c *gin.Context) { storageHTTP(c, "criteria.remove", "name", "", "", false, true) }
func setCriterion(c *gin.Context)    { storageHTTP(c, "criteria.set", "", "criterion", "", false, true) }
func getCriteria(c *gin.Context)     { storageHTTP(c, "criteria.get", "", "", "", true, false) }
func removeLocalStorageVals(c *gin.Context) {
	storageHTTP(c, "local.remove", "", "", "removeLocalStorageVals", false, true)
}
func setLocalStorageVal(c *gin.Context) {
	storageHTTP(c, "local.setValue", "key", "val", "setLocalStorageVal", false, true)
}
func setLocalStorage(c *gin.Context) {
	storageHTTP(c, "local.set", "", "val", "setLocalStorage", false, true)
}
func getLocalStorage(c *gin.Context)   { storageHTTP(c, "local.get", "", "", "", true, false) }
func getOutlineStorage(c *gin.Context) { storageHTTP(c, "outline.get", "docID", "", "", true, true) }
func setOutlineStorage(c *gin.Context) {
	storageHTTP(c, "outline.set", "docID", "val", "", false, true)
}
func removeOutlineStorage(c *gin.Context) {
	storageHTTP(c, "outline.remove", "docID", "", "", false, true)
}
func updateRecentDocViewTime(c *gin.Context) {
	storageHTTP(c, "recent.view", "rootID", "", "", false, true)
}
func updateRecentDocOpenTime(c *gin.Context) {
	storageHTTP(c, "recent.open", "rootID", "", "", false, true)
}
func updateRecentDocCloseTime(c *gin.Context) {
	storageHTTP(c, "recent.close", "rootID", "", "", false, true)
}
