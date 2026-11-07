// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIncrementalIndexesRejectMalformedPathsBeforeOwnerWork(t *testing.T) {
	for _, remove := range []bool{false, true} {
		for _, body := range []string{`{}`, `{"paths":null}`, `{"paths":"bad"}`, `{"paths":[42]}`, `{"paths":["ok",false]}`} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/filetree/upsertIndexes", bytes.NewBufferString(body))
			c.Request.Header.Set("Content-Type", "application/json")
			incrementalIndexes(c, remove)
			var reply struct {
				Code int
				Data interface{}
			}
			if e := json.Unmarshal(w.Body.Bytes(), &reply); e != nil || reply.Code != -1 || reply.Data != nil {
				t.Fatal(body, w.Body.String(), e)
			}
		}
	}
}
