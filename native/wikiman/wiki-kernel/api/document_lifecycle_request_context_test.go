package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDocumentLifecycleHandlersUseRequestContext(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		handler func(*gin.Context)
	}{
		{"move", `{"fromPaths":["/20261103200000-life001.docj"],"toNotebook":"20261028140000-refbox1","toPath":"/","callback":null}`, moveDocs},
		{"remove", `{"notebook":"20261028140000-refbox1","path":"/20261103200000-life001.docj"}`, removeDoc},
		{"remove-batch", `{"paths":["/20261103200000-life001.docj"]}`, removeDocs},
		{"rename", `{"notebook":"20261028140000-refbox1","path":"/20261103200000-life001.docj","title":"new"}`, renameDoc},
		{"restore", `{"notebook":"20261028140000-refbox1","historyPath":"/history/20261103200000-life001.docj"}`, rollbackDocHistory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/document/"+test.name, bytes.NewBufferString(test.body)).WithContext(ctx)
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
