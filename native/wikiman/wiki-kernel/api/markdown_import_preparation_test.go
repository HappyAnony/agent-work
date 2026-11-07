// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMarkdownImportHTTPPreparationUsesRequestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/import/importStdMd", strings.NewReader(`{"notebook":"20261031170000-box0001","localPath":"/missing/input.md","toPath":"/"}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	importStdMd(c)
	var reply struct {
		Code int
		Msg  string
		Data interface{}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || w.Code != 200 || reply.Code != -1 || reply.Msg != context.Canceled.Error() || reply.Data != nil {
		t.Fatal(w.Body.String(), err)
	}
}
