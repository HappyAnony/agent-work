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

func TestHTMLDesktopHandlersUseRequestContext(t *testing.T) {
	for _, markdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "html", true: "markdown"}[markdown], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"id":"20261101170000-desktop","savePath":"/tmp/selected-export","pdf":false}`)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			if markdown {
				exportMdHTML(c)
			} else {
				exportHTML(c)
			}
			var result struct {
				Code int
				Data interface{}
			}
			if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil || result.Code != -1 || result.Data != nil || w.Code != 200 {
				t.Fatal(w.Body.String(), e)
			}
		})
	}
}
