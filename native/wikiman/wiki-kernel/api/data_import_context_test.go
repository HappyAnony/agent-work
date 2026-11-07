// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type dataImportRunner struct{ run func(io.Reader) error }

func (d dataImportRunner) ImportDataReader(r io.Reader) error { return d.run(r) }
func TestDataImportHTTPSuccessNoDestinationAndMultipartCleanup(t *testing.T) {
	c, w, form, dir := importCleanupRequest(t)
	delete(form.Value, "notebook")
	delete(form.Value, "toPath")
	calls := 0
	importDataWithCapture(c, func(context.Context) (dataImportHandler, error) {
		return dataImportRunner{func(r io.Reader) error {
			calls++
			n, err := io.Copy(io.Discard, r)
			if n == 0 {
				t.Error("upload lost")
			}
			return err
		}}, nil
	})
	var ret struct {
		Code int
		Data interface{}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || w.Code != 200 || ret.Code != 0 || ret.Data != nil || calls != 1 {
		t.Fatal(w.Body.String(), err, calls)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("multipart leaked", entries, err)
	}
}
func TestDataImportHTTPCanceledCapturePrecedesBodyParsing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, w := ginDataImportRequest()
	c.Request = c.Request.WithContext(ctx)
	importData(c)
	if c.Request.MultipartForm != nil {
		t.Fatal("canceled capture consumed multipart")
	}
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || w.Code != 200 || ret.Code != -1 || ret.Msg != context.Canceled.Error() {
		t.Fatal(w.Body.String(), err)
	}
}

// Keep request bodies independent of any global environment or actual store.
func ginDataImportRequest() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/import/importData", strings.NewReader("unparsed"))
	return c, w
}
func TestDataImportHTTPReaderAndPrimaryCloseErrorsAreReturned(t *testing.T) {
	c, w, form, _ := importCleanupRequest(t)
	defer form.RemoveAll()
	importDataWithCapture(c, func(context.Context) (dataImportHandler, error) {
		return dataImportRunner{func(r io.Reader) error {
			if err := r.(io.Closer).Close(); err != nil {
				t.Fatal(err)
			}
			return errors.New("primary data import failure")
		}}, nil
	})
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 || !strings.Contains(ret.Msg, "primary data import failure") || !strings.Contains(ret.Msg, "upload close:") {
		t.Fatal(w.Body.String(), err)
	}
}
func TestDataImportHTTPMultipartCleanupFailureJoinsPrimary(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	c, w, form, dir := importCleanupRequest(t)
	defer func() { os.Chmod(dir, 0700); form.RemoveAll() }()
	importDataWithCapture(c, func(context.Context) (dataImportHandler, error) {
		return dataImportRunner{func(r io.Reader) error {
			io.Copy(io.Discard, r)
			os.Chmod(dir, 0500)
			return errors.New("primary data failure")
		}}, nil
	})
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 || !strings.Contains(ret.Msg, "primary data failure") || !strings.Contains(ret.Msg, "multipart cleanup:") {
		t.Fatal(w.Body.String(), err)
	}
}
func TestDataImportHTTPMissingFileAndMalformedBodyDoNotInvoke(t *testing.T) {
	for _, kind := range []string{"missing", "malformed", "capture"} {
		t.Run(kind, func(t *testing.T) {
			c, w, form, _ := importCleanupRequest(t)
			defer form.RemoveAll()
			if kind == "missing" {
				delete(form.File, "file")
			}
			if kind == "malformed" {
				c.Request = httptest.NewRequest("POST", "/api/import/importData", strings.NewReader("broken multipart"))
				c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
			}
			called := false
			importDataWithCapture(c, func(context.Context) (dataImportHandler, error) {
				if kind == "capture" {
					return nil, errors.New("owner capture failed")
				}
				return dataImportRunner{func(io.Reader) error { called = true; return nil }}, nil
			})
			var ret struct{ Code int }
			if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 || w.Code != 200 || called {
				t.Fatal(w.Body.String(), err, called)
			}
		})
	}
}
