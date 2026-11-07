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

func TestHTMLHandoffHandlersUseRequestContext(t *testing.T) {
	for _, kind := range []string{"html", "markdown", "browser"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			body := `{"id":"20261101140000-htmlmd1","savePath":"","pdf":false,"folder":"html-test","html":"wrapper","name":"test"}`
			c.Request = httptest.NewRequest("POST", "/", bytes.NewBufferString(body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			switch kind {
			case "html":
				exportHTML(c)
			case "markdown":
				exportMdHTML(c)
			case "browser":
				exportBrowserHTML(c)
			}
			var r struct {
				Code int
				Data interface{}
			}
			if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil || w.Code != 200 || r.Code != -1 || r.Data != nil {
				t.Fatal(w.Body.String(), err)
			}
		})
	}
}
