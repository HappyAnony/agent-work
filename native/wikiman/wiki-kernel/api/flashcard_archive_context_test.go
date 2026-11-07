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

func TestArchiveExportHandlerCanceledContextRetainsOriginalFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/export/exportSY", bytes.NewBufferString(`{"id":"20261031130000-block01"}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	exportSY(c)
	var reply struct {
		Code int
		Data map[string]string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || reply.Code != 0 || reply.Data["zip"] != "" || reply.Data["name"] != "" {
		t.Fatal(w.Body.String(), err)
	}
}

func TestArchiveNotebookExportHandlerCanceledContextRetainsOriginalFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/export/exportNotebookSY", bytes.NewBufferString(`{"id":"20261031150000-box0001"}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	exportNotebookSY(c)
	var reply struct {
		Code int
		Data map[string]string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || reply.Code != 0 || reply.Data["zip"] != "" {
		t.Fatal(w.Body.String(), err)
	}
}
