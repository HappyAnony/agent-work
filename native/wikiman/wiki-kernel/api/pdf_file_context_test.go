package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
	"wiki-kernel/model"
)

func TestPDFProcessHandlerValidatesAndUsesRequestContext(t *testing.T) {
	bodies := []string{`{}`, `{"id":3,"path":"/tmp/input.pdf","removeAssets":false,"watermark":false}`, `{"id":"id","path":null,"removeAssets":false,"watermark":false}`, `{"id":"id","path":"/tmp/input.pdf","removeAssets":"false","watermark":false}`, `{"id":"id","path":"/tmp/input.pdf","removeAssets":false,"watermark":[]}`, `{"id":"id","path":"/tmp/input.pdf","removeAssets":false,"watermark":false,"merge":"true"}`, `{"id":"20261101170000-desktop","path":"/tmp/input.pdf","removeAssets":false,"watermark":false}`}
	for _, body := range bodies {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", bytes.NewBufferString(body)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		processPDF(c)
		var result struct {
			Code int
			Data interface{}
		}
		if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil || w.Code != 200 || result.Code != -1 || result.Data != nil {
			t.Fatal(w.Body.String(), e)
		}
	}
}
func TestPDFProcessHandlerCommittedFailurePayload(t *testing.T) {
	if pdfProcessFailureData(errors.New("before commit")) != nil {
		t.Fatal("precommit payload changed")
	}
	e := errors.Join(&model.PDFProcessError{Code: "earlier"}, &model.PDFProcessError{Code: "late", Committed: true})
	data := pdfProcessFailureData(e)
	encoded, _ := json.Marshal(data)
	if string(encoded) != `{"committed":true}` {
		t.Fatal(string(encoded))
	}
}
