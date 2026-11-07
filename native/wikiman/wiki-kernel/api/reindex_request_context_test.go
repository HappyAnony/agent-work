package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReindexHandlersUseRequestContext(t *testing.T) {
	for _, test := range []struct {
		name    string
		handler func(*gin.Context)
	}{
		{"history", reindexHistory},
		{"asset-content", fullReindexAssetContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/reindex/"+test.name, nil).WithContext(ctx)
			test.handler(c)
			var result struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Code != -1 || result.Msg != context.Canceled.Error() {
				t.Fatalf("result = %#v, body = %s", result, w.Body.String())
			}
		})
	}
}
