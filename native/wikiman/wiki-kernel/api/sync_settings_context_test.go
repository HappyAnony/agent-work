package api

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncSettingsHTTPContextAndTypeFailures(t *testing.T) {
	for _, tt := range []struct {
		handler func(*gin.Context)
		body    string
	}{{setSyncEnable, `{"enabled":true}`}, {setSyncGenerateConflictDoc, `{"enabled":true}`}, {setSyncInterval, `{"interval":40}`}, {setSyncPerception, `{"enabled":false}`}, {setSyncMode, `{"mode":2}`}, {setSyncProvider, `{"provider":4}`}, {setSyncProviderS3, `{"s3":{"bucket":"x"}}`}, {setSyncProviderWebDAV, `{"webdav":{}}`}, {setSyncProviderLocal, `{"local":{}}`}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.body)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		tt.handler(c)
		var ret struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 || ret.Msg != context.Canceled.Error() {
			t.Fatal(w.Body.String(), err)
		}
	}
}
