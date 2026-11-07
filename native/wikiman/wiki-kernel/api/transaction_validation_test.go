package api

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransactionHTTPRejectsMalformedEnvelopeBeforeRuntime(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"reqId":"bad","app":"a","session":"s","transactions":[]}`,
		`{"reqId":1.5,"app":"a","session":"s","transactions":[]}`,
		`{"reqId":1,"app":1,"session":"s","transactions":[]}`,
		`{"reqId":1,"app":"a","session":[],"transactions":[]}`,
		`{"reqId":1,"app":"a","session":"s","transactions":null}`,
		`{"reqId":1,"app":"a","session":"s","transactions":[null]}`,
		`{"reqId":1,"app":"a","session":"s","transactions":[{"doOperations":[null]}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			performTransactions(c)
			var result struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != -1 {
				t.Fatalf("invalid response: %s %v", w.Body.String(), err)
			}
		})
	}
}
