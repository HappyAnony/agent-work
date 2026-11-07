//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wiki-kernel/util"
)

func extensionFixture(t *testing.T) (*extensionCopyContext, string, string) {
	t.Helper()
	m, box, _ := markdownSourceFixture(t)
	owner := m.archive.binding.scope.commit.r.owner
	owner.m.Lock()
	owner.Editor.Markdown.InlineAsterisk = true
	owner.Editor.Markdown.InlineMath = true
	owner.m.Unlock()
	p, err := captureExtensionCopy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return p, box, m.archive.binding.scope.commit.r.data
}
func extensionRequest(t *testing.T, fields map[string]string, files map[string][]byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := w.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range files {
		out, err := w.CreateFormFile(key, "upload.bin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = out.Write(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/extension/copy", bytes.NewReader(body.Bytes()))
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}

type extensionFaultBody struct {
	io.Reader
	onRead            func()
	readErr, closeErr error
	reads, closes     int
}

func (b *extensionFaultBody) Read(p []byte) (int, error) {
	b.reads++
	if b.onRead != nil {
		fn := b.onRead
		b.onRead = nil
		fn()
	}
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.Reader.Read(p)
}
func (b *extensionFaultBody) Close() error      { b.closes++; return b.closeErr }
func extensionEmpty(r ExtensionCopyResult) bool { return r == ExtensionCopyResult{} }

func TestExtensionCopyCapturesParserBeforeMultipartRead(t *testing.T) {
	p, _, _ := extensionFixture(t)
	req := extensionRequest(t, map[string]string{"dom": "<p> <strong>Captured bold</strong></p><table><tr><td>A</td><td>B</td></tr></table><iframe src=\"https://example.com\">\ninside\r\n</iframe>"}, nil)
	original := req.Body
	owner := p.preparation.archive.binding.scope.commit.r.owner
	old := util.MarkdownSettings
	poison := *old
	poison.InlineAsterisk = false
	poison.InlineMath = false
	body := &extensionFaultBody{Reader: original, onRead: func() {
		owner.m.Lock()
		owner.Editor.Markdown.InlineAsterisk = false
		owner.Editor.Markdown.InlineMath = false
		owner.m.Unlock()
		util.MarkdownSettings = &poison
	}}
	req.Body = body
	defer func() { util.MarkdownSettings = old }()
	got, err := p.copyRequest(req, 1)
	if err != nil || !strings.Contains(got.Markdown, `<span data-type="strong">Captured bold</span>`) || !strings.Contains(got.Markdown, "|A|B|") || strings.Contains(got.Markdown, "\ninside") || body.closes != 0 {
		t.Fatal(got, err, body.closes)
	}
	if req.Body != body {
		t.Fatal("request body ownership changed")
	}
	// HTML's plain-dollar escaping is independent of Markdown settings; verify
	// the captured parser against the original raw-article path and a fresh control.
	p.fetch = func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("Raw math $x$")), nil
	}
	remoteRequest := func() *http.Request {
		return extensionRequest(t, map[string]string{"dom": "<p>fallback</p>", "href": "https://ld246.com/article/123"}, nil)
	}
	captured, err := p.copyRequest(remoteRequest(), 1)
	if err != nil || !captured.WithMath {
		t.Fatal("captured parser lost math", captured, err)
	}
	fresh, err := captureExtensionCopy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fresh.fetch = p.fetch
	current, err := fresh.copyRequest(remoteRequest(), 1)
	if err != nil || current.WithMath {
		t.Fatal("fresh parser ignored disabled math", current, err)
	}
}
func TestExtensionCopyUploadsMappingNotebookFallbackAndScratchCleanup(t *testing.T) {
	for _, kind := range []string{"notebook", "missing", "file", "global"} {
		t.Run(kind, func(t *testing.T) {
			p, box, data := extensionFixture(t)
			folder := filepath.Join(data, "assets")
			switch kind {
			case "notebook":
				folder = filepath.Join(data, box, "assets")
				if err := os.MkdirAll(folder, 0755); err != nil {
					t.Fatal(err)
				}
			case "file":
				if err := os.WriteFile(filepath.Join(data, box, "assets"), []byte("not folder"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			image := []byte("original synthetic image bytes")
			address := "https://example.com/picture.png"
			fields := map[string]string{"dom": `<p> Original text</p><img src="` + address + `" alt="plain">`}
			if kind != "global" {
				fields["notebook"] = box
			}
			key := url.PathEscape(address + "?imageView2/2/interlace/1/format/webp")
			req := extensionRequest(t, fields, map[string][]byte{key: image})
			got, err := p.copyRequest(req, 1)
			if err != nil || !strings.Contains(got.Markdown, "assets/picture-") || strings.Contains(got.Markdown, address) || !strings.Contains(got.Markdown, "Original text") {
				t.Fatal(got, err)
			}
			entries, err := os.ReadDir(folder)
			if err != nil || len(entries) != 1 {
				t.Fatal(entries, err)
			}
			saved, err := os.ReadFile(filepath.Join(folder, entries[0].Name()))
			if err != nil || !bytes.Equal(saved, image) {
				t.Fatal(err)
			}
			headers := req.MultipartForm.File[key]
			if len(headers) != 1 {
				t.Fatal("multipart header missing")
			}
			input, err := headers[0].Open()
			if err == nil {
				input.Close()
				t.Fatal("multipart scratch file remains")
			}
		})
	}
}
func TestExtensionCopyUploadReadAndCloseFailuresJoinAndSuppressResult(t *testing.T) {
	p, _, data := extensionFixture(t)
	readErr, closeErr := errors.New("upload read"), errors.New("upload close")
	input := &extensionFaultBody{readErr: readErr, closeErr: closeErr}
	p.openUpload = func(*multipart.FileHeader) (io.ReadCloser, error) { return input, nil }
	req := extensionRequest(t, map[string]string{"dom": "<p>No partial result</p>"}, map[string][]byte{"https://example.com/image.png": []byte("upload")})
	got, err := p.copyRequest(req, 1)
	if !errors.Is(err, readErr) || !errors.Is(err, closeErr) || !extensionEmpty(got) || input.closes != 1 {
		t.Fatal(got, err, input.closes)
	}
	if entries, e := os.ReadDir(filepath.Join(data, "assets")); e != nil || len(entries) != 0 {
		t.Fatal(entries, e)
	}
}
func TestExtensionCopyUploadOpenAndTargetPermissionFailures(t *testing.T) {
	for _, kind := range []string{"open", "write"} {
		t.Run(kind, func(t *testing.T) {
			p, _, data := extensionFixture(t)
			if kind == "open" {
				p.openUpload = func(*multipart.FileHeader) (io.ReadCloser, error) { return nil, errors.New("upload open") }
			} else {
				if os.Geteuid() == 0 {
					t.Skip("root bypasses mode bits")
				}
				folder := filepath.Join(data, "assets")
				os.MkdirAll(folder, 0755)
				os.Chmod(folder, 0500)
				defer os.Chmod(folder, 0755)
			}
			got, err := p.copyRequest(extensionRequest(t, map[string]string{"dom": "<p>No partial success</p>"}, map[string][]byte{"https://example.com/a.png": []byte("upload")}), 1)
			if err == nil || !extensionEmpty(got) {
				t.Fatal(got, err)
			}
		})
	}
}
func TestExtensionCopyRootAndFormCleanupErrorsJoin(t *testing.T) {
	p, _, _ := extensionFixture(t)
	rootErr, cleanupErr := errors.New("extension root close"), errors.New("extension form cleanup")
	p.preparation.archive.closeRoot = func(root *os.Root) error { return errors.Join(root.Close(), rootErr) }
	cleanups := 0
	p.removeForm = func(form *multipart.Form) error { cleanups++; return errors.Join(form.RemoveAll(), cleanupErr) }
	got, err := p.copyRequest(extensionRequest(t, map[string]string{"dom": "<p>Must not escape close errors</p>"}, nil), 1)
	if !errors.Is(err, rootErr) || !errors.Is(err, cleanupErr) || !extensionEmpty(got) || cleanups != 1 {
		t.Fatal(got, err, cleanups)
	}
}
func TestExtensionCopyMissingDOMMalformedFormsAndUnsafeNotebooks(t *testing.T) {
	for _, kind := range []string{"dom", "form", "notebook", "notebook-link"} {
		t.Run(kind, func(t *testing.T) {
			p, box, data := extensionFixture(t)
			fields := map[string]string{"dom": "<p>Must fail</p>"}
			if kind == "dom" {
				delete(fields, "dom")
			}
			if kind == "notebook" {
				fields["notebook"] = "../outside"
			}
			if kind == "notebook-link" {
				fields["notebook"] = box
				os.Symlink(t.TempDir(), filepath.Join(data, box, "assets"))
			}
			req := extensionRequest(t, fields, nil)
			if kind == "form" {
				req.Header.Set("Content-Type", "application/json")
			}
			got, err := p.copyRequest(req, 1)
			if err == nil || !extensionEmpty(got) {
				t.Fatal(got, err)
			}
		})
	}
}
func TestExtensionCopyRevocationBeforeAndDuringMultipartRead(t *testing.T) {
	for _, kind := range []string{"cancel-before", "cancel-during", "readonly", "owner"} {
		t.Run(kind, func(t *testing.T) {
			p, _, _ := extensionFixture(t)
			ctx, cancel := context.WithCancel(p.preparation.ctx)
			defer cancel()
			p.preparation.ctx = ctx
			p.preparation.archive.ctx = ctx
			req := extensionRequest(t, map[string]string{"dom": "<p>Revoked</p>"}, nil)
			body := &extensionFaultBody{Reader: req.Body}
			if kind == "cancel-before" {
				cancel()
			} else if kind == "cancel-during" {
				body.onRead = cancel
			} else if kind == "readonly" {
				owner := p.preparation.archive.binding.scope.commit.r.owner
				body.onRead = func() { owner.m.Lock(); owner.persistenceReadOnly = true; owner.m.Unlock() }
				defer func() { owner.m.Lock(); owner.persistenceReadOnly = false; owner.m.Unlock() }()
			} else {
				old := Conf
				defer func() { Conf = old }()
				body.onRead = func() { Conf = NewAppConf() }
			}
			req.Body = body
			got, err := p.copyRequest(req, 1)
			if err == nil || !extensionEmpty(got) || kind == "cancel-before" && body.reads != 0 {
				t.Fatal(got, err, body.reads)
			}
		})
	}
}
func TestExtensionCopyRemoteArticleFormattingAndPartClip(t *testing.T) {
	for _, host := range []string{"ld246.com", "liuyun.io"} {
		t.Run(host, func(t *testing.T) {
			p, _, _ := extensionFixture(t)
			closed := &extensionFaultBody{Reader: strings.NewReader("    code line\n\nmath $x$\n")}
			calls := 0
			p.fetch = func(ctx context.Context, address string) (io.ReadCloser, error) {
				calls++
				if ctx != p.preparation.ctx || address != "https://"+host+"/article/raw/123" {
					t.Fatal(address)
				}
				return closed, nil
			}
			got, err := p.copyRequest(extensionRequest(t, map[string]string{"dom": "<p>HTML fallback</p>", "href": "https://" + host + "/article/123/path?q=1#frag"}, nil), 1)
			if err != nil || !got.WithMath || !strings.Contains(got.Markdown, "```") || !strings.Contains(got.Markdown, "code line") || strings.Contains(got.Markdown, "HTML fallback") || closed.closes != 1 {
				t.Fatal(got, err, closed.closes)
			}
			part, err := p.copyRequest(extensionRequest(t, map[string]string{"dom": "<p>Part clip</p>", "href": "https://" + host + "/article/123", "clipType": "part"}, nil), 1)
			if err != nil || !strings.Contains(part.Markdown, "Part clip") || calls != 1 {
				t.Fatal(part, err, calls)
			}
		})
	}
}
func TestExtensionCopyRemoteFallbackReadCloseAndCancellation(t *testing.T) {
	for _, kind := range []string{"fallback", "read-close", "close", "cancel", "owner", "fetch-body-error"} {
		t.Run(kind, func(t *testing.T) {
			p, _, _ := extensionFixture(t)
			ctx, cancel := context.WithCancel(p.preparation.ctx)
			defer cancel()
			p.preparation.ctx = ctx
			p.preparation.archive.ctx = ctx
			readErr, closeErr := errors.New("remote read"), errors.New("remote close")
			body := &extensionFaultBody{Reader: strings.NewReader("Remote content")}
			if kind == "read-close" {
				body.readErr = readErr
				body.closeErr = closeErr
			}
			if kind == "close" {
				body.closeErr = closeErr
			}
			old := Conf
			defer func() { Conf = old }()
			p.fetch = func(ctx context.Context, address string) (io.ReadCloser, error) {
				switch kind {
				case "fallback":
					return nil, readErr
				case "cancel":
					cancel()
					return body, nil
				case "owner":
					Conf = NewAppConf()
					return body, nil
				case "fetch-body-error":
					body.closeErr = closeErr
					return body, readErr
				}
				return body, nil
			}
			got, err := p.copyRequest(extensionRequest(t, map[string]string{"dom": "<p>Fallback content</p>", "href": "https://ld246.com/article/123"}, nil), 1)
			if kind == "fallback" {
				if err != nil || !strings.Contains(got.Markdown, "Fallback content") {
					t.Fatal(got, err)
				}
				return
			}
			if err == nil || !extensionEmpty(got) || body.closes != 1 {
				t.Fatal(got, err, body.closes)
			}
			if kind == "read-close" || kind == "fetch-body-error" {
				if !errors.Is(err, readErr) || !errors.Is(err, closeErr) {
					t.Fatal(err)
				}
			}
		})
	}
}
func TestExtensionCopyMIMEInferenceAndInvalidUploadURLs(t *testing.T) {
	p, _, data := extensionFixture(t)
	req := extensionRequest(t, map[string]string{"dom": `<p>Format</p><img src="https://example.com/noextension">`}, map[string][]byte{"https://example.com/noextension": []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"), "http://[bad": []byte("skip"), "invalid%ZZ": []byte("skip")})
	got, err := p.copyRequest(req, 1)
	if err != nil || !strings.Contains(got.Markdown, ".svg") {
		t.Fatal(got, err)
	}
	entries, err := os.ReadDir(filepath.Join(data, "assets"))
	if err != nil || len(entries) != 1 {
		t.Fatal(entries, err)
	}
}

func TestExtensionCopyHTTPTransportContextAndRawBody(t *testing.T) {
	p, _, _ := extensionFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("Raw markdown **body**")) }))
	defer server.Close()
	input, err := p.fetch(p.preparation.ctx, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := p.readAndClose(input)
	if err != nil || string(raw) != "Raw markdown **body**" {
		t.Fatal(string(raw), err)
	}
	ctx, cancel := context.WithCancel(p.preparation.ctx)
	cancel()
	input, err = p.fetch(ctx, server.URL)
	if input != nil {
		input.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
