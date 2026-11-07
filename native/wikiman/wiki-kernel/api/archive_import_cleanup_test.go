// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type importCleanupRunner struct{ run func(io.Reader) error }

func (r importCleanupRunner) ImportReader(input io.Reader, box, to string) error { return r.run(input) }
func importCleanupRequest(t *testing.T) (*gin.Context, *httptest.ResponseRecorder, *multipart.Form, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("notebook", "20261101090000-box0001")
	writer.WriteField("toPath", "/")
	file, _ := writer.CreateFormFile("file", "same-name.zip")
	file.Write(bytes.Repeat([]byte("upload"), 512))
	writer.Close()
	request := httptest.NewRequest("POST", "/api/import/importSY", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(1); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = request
	return c, response, request.MultipartForm, dir
}
func TestArchiveImportCleanupHTTPPrimaryAndMultipartErrorsPreserved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permissions")
	}
	c, w, form, dir := importCleanupRequest(t)
	defer func() { os.Chmod(dir, 0700); form.RemoveAll() }()
	importSYWithCapture(c, func(context.Context) (archiveImportHandler, error) {
		return importCleanupRunner{run: func(input io.Reader) error {
			io.Copy(io.Discard, input)
			os.Chmod(dir, 0500)
			return errors.New("primary import failure")
		}}, nil
	})
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || w.Code != 200 || ret.Code != -1 || !strings.Contains(ret.Msg, "primary import failure") || !strings.Contains(ret.Msg, "multipart cleanup:") {
		t.Fatal(w.Body.String(), err)
	}
}
func TestArchiveImportCleanupHTTPReaderCloseFailureNotSuccess(t *testing.T) {
	c, w, form, _ := importCleanupRequest(t)
	defer form.RemoveAll()
	importSYWithCapture(c, func(context.Context) (archiveImportHandler, error) {
		return importCleanupRunner{run: func(input io.Reader) error { return input.(io.Closer).Close() }}, nil
	})
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || ret.Code != -1 || !strings.Contains(ret.Msg, "upload close:") {
		t.Fatal(w.Body.String(), err)
	}
}
func TestArchiveImportCleanupHTTPSuccessRetainsOriginalEnvelope(t *testing.T) {
	c, w, form, dir := importCleanupRequest(t)
	defer form.RemoveAll()
	count := 0
	importSYWithCapture(c, func(context.Context) (archiveImportHandler, error) {
		return importCleanupRunner{run: func(input io.Reader) error { count++; _, err := io.Copy(io.Discard, input); return err }}, nil
	})
	var ret struct {
		Code int
		Msg  string
		Data interface{}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || w.Code != 200 || ret.Code != 0 || ret.Data != nil || count != 1 {
		t.Fatal(w.Body.String(), err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("multipart temp leaked")
	}
}

func TestArchiveImportCleanupHTTPParsedFormWithQueryErrorStillRemoved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("notebook", "20261101090000-box0001")
	writer.WriteField("toPath", "/")
	file, _ := writer.CreateFormFile("file", "same-name.zip")
	file.Write(bytes.Repeat([]byte("upload"), 512))
	writer.Close()
	w := httptest.NewRecorder()
	c, engine := gin.CreateTestContext(w)
	engine.MaxMultipartMemory = 1
	c.Request = httptest.NewRequest("POST", "/api/import/importSY?bad=%zz", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	called := false
	importSYWithCapture(c, func(context.Context) (archiveImportHandler, error) {
		return importCleanupRunner{run: func(io.Reader) error { called = true; return nil }}, nil
	})
	var ret struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &ret); err != nil || w.Code != 200 || ret.Code != -1 || !strings.Contains(ret.Msg, "invalid URL escape") || called {
		t.Fatal(w.Body.String(), err)
	}
	if c.Request.MultipartForm == nil || len(c.Request.MultipartForm.File["file"]) != 1 {
		t.Fatal("fixture did not finish multipart parsing")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("parsed form with query error leaked", entries)
	}
}
