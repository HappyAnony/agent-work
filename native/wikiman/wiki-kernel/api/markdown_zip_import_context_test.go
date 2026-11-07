// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"wiki-kernel/model"
)

type markdownZIPRunner struct {
	run func(io.Reader, string, string, string) error
}

func (r markdownZIPRunner) ImportZIPReader(input io.Reader, box, to, name string) error {
	return r.run(input, box, to, name)
}
func TestMarkdownZIPHTTPSuccessAndMultipartCleanup(t *testing.T) {
	c, w, form, dir := importCleanupRequest(t)
	defer form.RemoveAll()
	calls := 0
	importZipMdWithCapture(c, func(context.Context) (markdownZIPImportHandler, error) {
		return markdownZIPRunner{func(r io.Reader, box, to, name string) error {
			calls++
			if box != "20261101090000-box0001" || to != "/" || name != "same-name.zip" {
				t.Fatal(box, to, name)
			}
			_, err := io.Copy(io.Discard, r)
			return err
		}}, nil
	})
	var ret struct {
		Code int
		Data interface{}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || w.Code != 200 || ret.Code != 0 || ret.Data != nil || calls != 1 {
		t.Fatal(w.Body.String(), err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("multipart residue", entries, err)
	}
}
func TestMarkdownZIPHTTPCapturePrecedesBodyParsing(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest("POST", "/api/import/importZipMd", strings.NewReader("malformed multipart")).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=broken")
	importZipMdWithCapture(c, func(ctx context.Context) (markdownZIPImportHandler, error) {
		return model.CaptureMarkdownZIPImport(ctx)
	})
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 || !strings.Contains(ret.Msg, "context canceled") || c.Request.MultipartForm != nil {
		t.Fatal(w.Body.String(), err)
	}
}
func TestMarkdownZIPHTTPReaderCloseAndPrimaryErrorsJoined(t *testing.T) {
	c, w, form, _ := importCleanupRequest(t)
	defer form.RemoveAll()
	importZipMdWithCapture(c, func(context.Context) (markdownZIPImportHandler, error) {
		return markdownZIPRunner{func(r io.Reader, _, _, _ string) error {
			if err := r.(io.Closer).Close(); err != nil {
				t.Fatal(err)
			}
			return errors.New("primary zip failure")
		}}, nil
	})
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 || !strings.Contains(ret.Msg, "primary zip failure") || !strings.Contains(ret.Msg, "upload close:") {
		t.Fatal(w.Body.String(), err)
	}
}
func TestMarkdownZIPHTTPMultipartCleanupFailurePreservesPrimary(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permissions")
	}
	c, w, form, dir := importCleanupRequest(t)
	defer func() { os.Chmod(dir, 0700); form.RemoveAll() }()
	importZipMdWithCapture(c, func(context.Context) (markdownZIPImportHandler, error) {
		return markdownZIPRunner{func(r io.Reader, _, _, _ string) error {
			io.Copy(io.Discard, r)
			if err := os.Chmod(dir, 0500); err != nil {
				t.Fatal(err)
			}
			return errors.New("primary zip failure")
		}}, nil
	})
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 || !strings.Contains(ret.Msg, "primary zip failure") || !strings.Contains(ret.Msg, "multipart cleanup:") {
		t.Fatal(w.Body.String(), err)
	}
}
func TestMarkdownZIPHTTPMissingFileAndDestination(t *testing.T) {
	for _, missing := range []string{"file", "destination"} {
		t.Run(missing, func(t *testing.T) {
			c, w, form, _ := importCleanupRequest(t)
			defer form.RemoveAll()
			if missing == "file" {
				form.File["file"] = nil
			} else {
				delete(form.Value, "toPath")
			}
			called := false
			importZipMdWithCapture(c, func(context.Context) (markdownZIPImportHandler, error) {
				return markdownZIPRunner{func(io.Reader, string, string, string) error { called = true; return nil }}, nil
			})
			var ret struct{ Code int }
			if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 || called {
				t.Fatal(w.Body.String(), err)
			}
		})
	}
}
