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

func TestMarkdownExportHandlersCanceledContextKeepsFallback(t *testing.T) {
	for _, item := range []struct {
		name, body string
		handler    func(*gin.Context)
		fallback   string
	}{{"single", `{"id":"20261101090000-doc0001"}`, exportMd, ""}, {"multiple", `{"ids":["20261101090000-doc0001"]}`, exportMds, ""}, {"notebook", `{"notebook":"20261101090000-box0001"}`, exportNotebookMd, "."}, {"epub", `{"id":"20261101090000-doc0001"}`, exportEPUB, ""}} {
		t.Run(item.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/export/"+item.name, bytes.NewBufferString(item.body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			item.handler(c)
			var ret struct {
				Code int
				Data map[string]string
			}
			if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || w.Code != 200 || ret.Code != 0 || ret.Data["zip"] != "" || ret.Data["name"] != item.fallback {
				t.Fatal(w.Body.String(), err)
			}
		})
	}
}
func TestMarkdownExportHandlerEmptySelectionDoesNotPanic(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/export/exportMds", bytes.NewBufferString(`{"ids":[]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	exportMds(c)
	var ret struct {
		Code int
		Data map[string]string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != 0 || ret.Data["name"] != "" || ret.Data["zip"] != "" {
		t.Fatal(w.Body.String(), err)
	}
}

func TestMarkdownContentHandlersUseRequestContextWithoutReadingGlobalDefaults(t *testing.T) {
	for _, item := range []struct {
		name, body string
		handler    func(*gin.Context)
	}{
		{"exportMdContent", `{"id":"20261101090000-doc0001"}`, exportMdContent},
		{"copyStdMarkdown", `{"id":"20261101090000-doc0001"}`, copyStdMarkdown},
	} {
		t.Run(item.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/"+item.name, bytes.NewBufferString(item.body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			item.handler(c)
			var result struct {
				Code int
				Data interface{}
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Code != -1 || result.Data != nil {
				t.Fatal(w.Body.String(), err)
			}
		})
	}
}
