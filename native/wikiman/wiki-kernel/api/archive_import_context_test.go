// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestArchiveImportHandlerCanceledAdmissionOriginalEnvelope(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/import/importSY", nil).WithContext(ctx)
	importSY(c)
	var reply struct {
		Code int
		Msg  string
		Data interface{}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil || reply.Code != -1 || reply.Msg != context.Canceled.Error() || w.Code != 200 {
		t.Fatal(w.Body.String(), err)
	}
}
