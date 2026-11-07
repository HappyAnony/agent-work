// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestImageConversionHTTPHTMLUsesCancellationAndFailureEnvelope(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/lute/html2BlockDOM", strings.NewReader(`{"dom":"<p>canceled</p>"}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	html2BlockDOM(c)
	var ret struct {
		Code int
		Msg  string
		Data interface{}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || w.Code != 200 || ret.Code != -1 || ret.Msg != context.Canceled.Error() || ret.Data != "Failed to convert" {
		t.Fatal(w.Body.String(), err)
	}
}
func TestImageConversionHTTPExtensionUsesCancellation(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("dom", "<p>canceled</p>"); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/extension/copy", &body).WithContext(ctx)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	unread := &extensionUnreadBody{}
	c.Request.Body = unread
	extensionCopy(c)
	if unread.reads != 0 {
		t.Fatal("canceled clipping read its request")
	}
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || w.Code != 200 || ret.Code != -1 || ret.Msg != context.Canceled.Error() {
		t.Fatal(w.Body.String(), err)
	}
}

type extensionUnreadBody struct{ reads int }

func (b *extensionUnreadBody) Read(p []byte) (int, error) { b.reads++; return 0, context.Canceled }
func (b *extensionUnreadBody) Close() error               { return nil }
