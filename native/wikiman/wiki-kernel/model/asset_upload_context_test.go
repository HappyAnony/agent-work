//go:build fts5

package model

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wiki-kernel/util"
)

func uploadRequest(t *testing.T, values map[string]string, names []string, bodies [][]byte) *http.Request {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for key, value := range values {
		if e := w.WriteField(key, value); e != nil {
			t.Fatal(e)
		}
	}
	for i, name := range names {
		f, e := w.CreateFormFile("file[]", name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(bodies[i]); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	request := httptest.NewRequest("POST", "/api/asset/upload", &b)
	request.Header.Set("Content-Type", w.FormDataContentType())
	return request
}

func TestAssetUploadActualMultipartNamesDedupAndCleanup(t *testing.T) {
	c, _, id, _, _ := referenceModelFixture(t)
	request := uploadRequest(t, map[string]string{"id": id}, []string{"hello[world].TXT", "same.TXT"}, [][]byte{[]byte("same original bytes"), []byte("same original bytes")})
	out, e := UploadAssetsRequest(context.Background(), request, 0)
	if e != nil {
		t.Fatal(e)
	}
	p, ok := out.SuccMap["hello[world].TXT"].(string)
	if !ok || !strings.HasPrefix(p, "assets/helloworld-") || !strings.HasSuffix(p, ".txt") || out.SuccMap["same.TXT"] != p {
		t.Fatal(out)
	}
	raw, e := os.ReadFile(filepath.Join(util.DataDir, p))
	if e != nil || string(raw) != "same original bytes" {
		t.Fatal(string(raw), e)
	}
	// The multipart parser spilled the uploads to disk; ownership includes their
	// actual removal, rather than only a cancellation flag or task pointer.
	for _, header := range request.MultipartForm.File["file[]"] {
		f, e := header.Open()
		if e == nil {
			f.Close()
			t.Fatal("multipart scratch retained")
		}
	}
	c.persistenceReadOnly = true
	refused := uploadRequest(t, nil, []string{"readonly.txt"}, [][]byte{[]byte("read only")})
	if _, e = UploadAssetsRequest(context.Background(), refused, 0); e == nil || refused.MultipartForm != nil {
		t.Fatal("readonly parsed or published request", e)
	}
}

func TestAssetUploadExplicitDirectoryAndDuplicatePrefix(t *testing.T) {
	_, _, _, _, _ = referenceModelFixture(t)
	existing := "custom-assets/same-20261104100000-asset01.txt"
	archiveWrite(t, util.DataDir, existing, []byte("previous bytes"))
	request := uploadRequest(t, map[string]string{"assetsDirPath": "custom-assets", "skipIfDuplicated": "true"}, []string{"same.TXT", "not-processed.txt"}, [][]byte{[]byte("different bytes"), []byte("must not write")})
	out, e := UploadAssetsRequest(context.Background(), request, 0)
	if e != nil || len(out.SuccMap) != 1 || out.SuccMap["same.TXT"] != existing {
		t.Fatal(out, e)
	}
	for _, selected := range []string{"../conf", "/absolute", "data\\assets"} {
		request = uploadRequest(t, map[string]string{"assetsDirPath": selected}, []string{"file.txt"}, [][]byte{[]byte("x")})
		if out, e = UploadAssetsRequest(context.Background(), request, 0); e == nil || len(out.SuccMap) > 0 {
			t.Fatal(selected, out, e)
		}
	}
	outside := t.TempDir()
	if e = os.Symlink(outside, filepath.Join(util.DataDir, "linked-assets")); e != nil {
		t.Fatal(e)
	}
	request = uploadRequest(t, map[string]string{"assetsDirPath": "linked-assets"}, []string{"file.txt"}, [][]byte{[]byte("x")})
	if _, e = UploadAssetsRequest(context.Background(), request, 0); e == nil {
		t.Fatal("linked destination admitted")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("outside modified")
	}
}

func TestAssetUploadCanceledBodyReadIsJoined(t *testing.T) {
	_, _, _, _, _ = referenceModelFixture(t)
	reader, writer := io.Pipe()
	defer writer.Close()
	request := httptest.NewRequest("POST", "/api/asset/upload", reader)
	request.Header.Set("Content-Type", "multipart/form-data; boundary=blocked")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := UploadAssetsRequest(ctx, request, 0); done <- e }()
	// A complete first line is accepted by the multipart parser; it then blocks
	// on this deliberately unfinished transport body until cancellation closes it.
	written := make(chan error, 1)
	go func() { _, e := io.WriteString(writer, "--blocked\r\n"); written <- e }()
	select {
	case e := <-written:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("parser did not read body")
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("body read not joined")
	}
}
