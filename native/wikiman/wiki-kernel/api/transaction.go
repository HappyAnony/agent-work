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
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/88250/gulu"
	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
	"wiki-kernel/util"
)

func performTransactions(c *gin.Context) {
	start := time.Now()
	ret := gulu.Ret.NewResult()
	defer c.JSON(http.StatusOK, ret)

	arg, ok := util.JsonArg(c, ret)
	if !ok {
		return
	}

	trans := arg["transactions"]
	requestID, idOK := arg["reqId"].(float64)
	app, appOK := arg["app"].(string)
	session, sessionOK := arg["session"].(string)
	_, batchOK := trans.([]interface{})
	if !idOK || math.IsNaN(requestID) || math.IsInf(requestID, 0) || math.Trunc(requestID) != requestID || requestID < math.MinInt64 || requestID >= float64(math.MaxInt64) || !appOK || !sessionOK || !batchOK {
		ret.Code = -1
		ret.Msg = "parses request failed"
		return
	}
	data, err := gulu.JSON.MarshalJSON(trans)
	if err != nil {
		ret.Code = -1
		ret.Msg = "parses request failed"
		return
	}

	timestamp := int64(requestID)
	var transactions []*model.Transaction
	if err = gulu.JSON.UnmarshalJSON(data, &transactions); err != nil {
		ret.Code = -1
		ret.Msg = "parses request failed"
		return
	}
	if err = model.ValidateTransactionsContext(c.Request.Context(), transactions); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}
	if !util.IsBooted() {
		ret.Code = -1
		ret.Msg = fmt.Sprintf(model.Conf.Language(74), int(util.GetBootProgress()))
		ret.Data = map[string]interface{}{"closeTimeout": 5000}
		return
	}
	for _, transaction := range transactions {
		transaction.Timestamp = timestamp
	}

	ctx := c.Request.Context()
	if err := model.PerformTransactionsContext(ctx, &transactions); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	ret.Data = transactions

	if err = pushTransactionsContext(ctx, app, session, transactions); err != nil {
		ret.Code = -1
		ret.Msg = err.Error()
		return
	}

	if model.IsMoveOutlineHeading(&transactions) {
		if retData := transactions[0].DoOperations[0].RetData; nil != retData {
			util.PushReloadDoc(retData.(string))
		}
	}

	elapsed := time.Now().Sub(start).Milliseconds()
	c.Header("Server-Timing", fmt.Sprintf("total;dur=%d", elapsed))
}

func pushTransactionsContext(ctx context.Context, app, session string, transactions []*model.Transaction) error {
	for _, tx := range transactions {
		if tx == nil {
			return fmt.Errorf("transaction_batch_invalid")
		}
	}
	pushMode := util.PushModeBroadcastExcludeSelf
	if 0 < len(transactions) && 0 < len(transactions[0].DoOperations) {
		if err := model.FlushTxQueueContext(ctx); err != nil { // 等待文件写入完成，后续渲染才能读取到最新的数据
			return err
		}

		action := transactions[0].DoOperations[0].Action
		isAttrViewTx := strings.Contains(strings.ToLower(action), "attrview")
		if isAttrViewTx && "setAttrViewName" != action {
			pushMode = util.PushModeBroadcast
		}
	}

	evt := util.NewCmdResult("transactions", 0, pushMode)
	evt.AppId = app
	evt.SessionId = session
	evt.Data = transactions
	for _, tx := range transactions {
		if err := tx.WaitForCommitContext(ctx); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	util.PushEvent(evt)
	return nil
}
