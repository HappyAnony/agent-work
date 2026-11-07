// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestReferenceRefreshRejectsInvalidIDWithoutStartingOwnerWork(t *testing.T) {
	for _, body := range []string{`{}`, `{"id":null}`, `{"id":42}`, `{"id":""}`} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/api/ref/refreshBacklink", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		refreshBacklink(c)
		var reply struct {
			Code int
			Data interface{}
		}
		if e := json.Unmarshal(w.Body.Bytes(), &reply); e != nil || reply.Code != -1 || reply.Data != nil {
			t.Fatal(body, w.Body.String(), e)
		}
	}
}
