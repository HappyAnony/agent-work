package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGraphHTTPRealRequestContext(t *testing.T) {
	for _, tt := range []struct {
		h    func(*gin.Context)
		body string
	}{{resetGraph, `{}`}, {resetLocalGraph, `{}`}, {getGraph, `{"k":"","conf":{}}`}, {getLocalGraph, `{"id":"20261028140002-refdef1","k":"","conf":{}}`}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.body)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		tt.h(c)
		var ret struct {
			Code int
			Msg  string
		}
		if e := json.Unmarshal(w.Body.Bytes(), &ret); e != nil || ret.Code != -1 || ret.Msg != context.Canceled.Error() {
			t.Fatal(w.Body.String(), e)
		}
	}
}
