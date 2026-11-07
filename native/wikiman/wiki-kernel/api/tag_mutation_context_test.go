package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTagHTTPMutationsCancelAndInvalidTypes(t *testing.T) {
	for _, test := range []struct {
		handler func(*gin.Context)
		body    string
		cancel  bool
	}{
		{removeTag, `{"label":"a"}`, true}, {renameTag, `{"oldLabel":"a","newLabel":"b"}`, true},
		{removeTag, `{"label":1}`, false}, {renameTag, `{"oldLabel":[],"newLabel":"b"}`, false},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		if test.cancel {
			cancel()
		}
		defer cancel()
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(test.body)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		test.handler(c)
		var result struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil || result.Code != -1 {
			t.Fatalf("response %s %v", w.Body.String(), e)
		}
		if test.cancel && result.Msg != context.Canceled.Error() {
			t.Fatal(result.Msg)
		}
	}
}
