package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAssetAttributeAndDailyHandlersUseRequestContext(t *testing.T) {
	const id = "20261104010101-abcdefg"
	tests := []struct {
		name    string
		body    string
		handler func(*gin.Context)
	}{
		{"asset-resolve", `{"path":"assets/missing.png"}`, resolveAssetPath},
		{"asset-copy", `{"src":"assets/missing.png","destDir":"data"}`, copyFile},
		{"annotation-set", `{"path":"assets/missing.png.sya","data":"x"}`, setFileAnnotation},
		{"annotation-get", `{"path":"assets/missing.png.sya"}`, getFileAnnotation},
		{"attribute-set", `{"id":"` + id + `","attrs":{"title":"value","memo":"value"}}`, setBlockAttrs},
		{"attribute-batch", `{"blockAttrs":[{"id":"` + id + `","attrs":{"title":"value"}}]}`, batchSetBlockAttrs},
		{"daily-append", `{"data":"<div></div>","dataType":"dom","notebook":"` + id + `"}`, appendDailyNoteBlock},
		{"daily-prepend", `{"data":"<div></div>","dataType":"dom","notebook":"` + id + `"}`, prependDailyNoteBlock},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/request-context/"+test.name, bytes.NewBufferString(test.body)).WithContext(ctx)
			c.Request.Header.Set("Content-Type", "application/json")
			test.handler(c)

			var result struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Code != -1 || !strings.Contains(result.Msg, context.Canceled.Error()) {
				t.Fatalf("result = %#v, body = %s", result, w.Body.String())
			}
		})
	}
}
