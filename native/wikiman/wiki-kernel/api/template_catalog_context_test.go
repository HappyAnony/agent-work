package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTemplateCatalogHTTPContextAndFieldTypes(t *testing.T) {
	for _, test := range []struct {
		handler func(*gin.Context)
		body    string
		cancel  bool
	}{
		{searchTemplate, `{"k":"x"}`, true}, {removeTemplate, `{"path":"/unused"}`, true},
		{searchTemplate, `{"k":1}`, false}, {removeTemplate, `{"path":[]}`, false},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		if test.cancel {
			cancel()
		}
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(test.body)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		test.handler(c)
		cancel()
		var ret struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 {
			t.Fatal(w.Body.String(), err)
		}
		if test.cancel && ret.Msg != context.Canceled.Error() {
			t.Fatal(ret.Msg)
		}
	}
}
