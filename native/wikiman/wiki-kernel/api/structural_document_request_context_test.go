package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStructuralDocumentHandlersUseRequestContext(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		handler func(*gin.Context)
	}{
		{"heading", `{"srcHeadingID":"heading","targetNoteBook":"box","targetPath":"/"}`, heading2Doc},
		{"list-item", `{"srcListItemID":"item","targetNoteBook":"box","targetPath":"/"}`, li2Doc},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/filetree/"+test.name, bytes.NewBufferString(test.body)).WithContext(ctx)
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
