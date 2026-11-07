package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtensionPreferencesHTTPRequestContext(t *testing.T) {
	for _, tt := range []struct {
		h    func(*gin.Context)
		body string
	}{{getSnippet, `{"type":"all","enabled":2}`}, {setSnippet, `{"snippets":[]}`}, {removeSnippet, `{"id":"x"}`}, {loadPetals, `{"frontend":"desktop"}`}, {setPetalEnabled, `{"packageName":"demo","enabled":true,"frontend":"desktop"}`}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.body)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		tt.h(c)
		var ret struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &ret); e != nil || ret.Code != -1 || !strings.Contains(ret.Msg, context.Canceled.Error()) {
			t.Fatal(w.Body.String(), e)
		}
	}
}
