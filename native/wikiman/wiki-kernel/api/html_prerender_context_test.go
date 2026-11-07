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

func TestHTMLPreRenderHandlerRequestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/export/exportPreviewHTML", bytes.NewBufferString(`{"id":"20261101123000-preroot","merge":true,"image":true}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	exportPreviewHTML(c)
	var result struct {
		Code int
		Data interface{}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Code != -1 || result.Data != nil {
		t.Fatal(w.Body.String(), err)
	}
}
