// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestAVReaderHandlersHonorCanceledRequestAndErrorEnvelopes(t *testing.T) {
	for _, item := range []struct {
		name, body string
		handler    gin.HandlerFunc
		code       int
	}{
		{"duplicate", `{"avID":"20261030210500-reader1"}`, duplicateAttributeViewBlock, -1},
		{"images", `{"id":"20261030210500-reader1"}`, getCurrentAttrViewImages, -1},
		{"preview", `{"avID":"20261030210500-reader1"}`, getAttributeViewAddingBlockDefaultValues, -1},
		{"block-keys", `{"id":"20261030210500-reader1"}`, getAttributeViewKeys, -1},
		{"to-items", `{"avID":"20261030210500-reader1","blockIDs":["missing"]}`, getAttributeViewItemIDsByBoundIDs, -1},
		{"to-blocks", `{"avID":"20261030210500-reader1","itemIDs":["missing"]}`, getAttributeViewBoundBlockIDsByItemIDs, -1},
	} {
		t.Run(item.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/av/reader", bytes.NewBufferString(item.body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			item.handler(c)
			var reply struct {
				Code int
				Msg  string
				Data interface{}
			}
			if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || reply.Code != item.code {
				t.Fatal(w.Body.String(), err)
			}
			if reply.Msg != context.Canceled.Error() || reply.Data != nil {
				t.Fatal("canceled request returned successful data", reply)
			}
		})
	}
}
