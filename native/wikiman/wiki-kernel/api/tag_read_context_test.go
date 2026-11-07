package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTagHTTPReadsContextAndInvalidTypes(t *testing.T) {
	for _, test := range []struct {
		handler func(*gin.Context)
		body    string
		cancel  bool
	}{
		{getTag, `{"app":"test"}`, true}, {searchTag, `{"k":"x"}`, true},
		{getTag, `{"app":"test","sort":"bad"}`, false}, {getTag, `{"app":"test","ignoreMaxListHint":1}`, false}, {searchTag, `{"k":[]}`, false},
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
			t.Fatalf("%s %v", w.Body.String(), e)
		}
		if test.cancel && result.Msg != context.Canceled.Error() {
			t.Fatal(result.Msg)
		}
	}
}
