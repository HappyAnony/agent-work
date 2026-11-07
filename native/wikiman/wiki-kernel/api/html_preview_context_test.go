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

func TestHTMLPreviewHandlerPropagatesRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/export/preview", bytes.NewBufferString(`{"id":"20261101110000-prvdoc1"}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	exportPreview(c)
	var result struct {
		Code int
		Data interface{}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Code != -1 || result.Data != nil {
		t.Fatal(w.Body.String(), err)
	}
}
