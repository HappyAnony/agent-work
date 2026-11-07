package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestResetAndShorthandHandlersUseRequestContext(t *testing.T) {
	for _, test := range []struct {
		name, body string
		handler    func(*gin.Context)
	}{
		{"reset-block-attrs", `{"id":"20261104120000-abcdefg","attrs":{"custom-test":"value"}}`, resetBlockAttrs},
		{"move-local-shorthands", `{"notebook":"20261104120000-abcdefg","path":""}`, moveLocalShorthands},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/"+test.name, bytes.NewBufferString(test.body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
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
