package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestBookmarkHTTPReadsPropagateCancellation(t *testing.T) {
	for _, handler := range []func(*gin.Context){getBookmark, getBookmarkLabels} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", nil).WithContext(ctx)
		handler(c)
		var result struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil || result.Code != -1 || result.Msg != context.Canceled.Error() {
			t.Fatalf("response %s %v", w.Body.String(), e)
		}
	}
}
