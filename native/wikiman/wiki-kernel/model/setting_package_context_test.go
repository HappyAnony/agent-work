//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/settingpack"
	"wiki-kernel/util"
)

func singleUploadRequest(t *testing.T, name string, raw []byte, values map[string]string) *http.Request {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for key, value := range values {
		if e := w.WriteField(key, value); e != nil {
			t.Fatal(e)
		}
	}
	member, e := w.CreateFormFile("file", name)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = member.Write(raw); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	request := httptest.NewRequest("POST", "/upload", &b)
	request.Header.Set("Content-Type", w.FormDataContentType())
	return request
}

func TestSettingPackageActualExportProviderRoundTripAndPrivatePolicy(t *testing.T) {
	c, _, p, _ := restoreFixture(t)
	ctx := context.Background()
	c.Sync.WebDAV = &conf.WebDAV{Endpoint: " example.test ", Username: "fixture", Password: "synthetic-password"}
	c.Sync.S3 = &conf.S3{Endpoint: " object.test ", Bucket: "fixture", AccessKey: "synthetic-access", SecretKey: "synthetic-secret"}
	for _, kind := range []string{"syncWebDAV", "syncS3"} {
		result, e := ExportSettingPackageContext(ctx, kind)
		if e != nil {
			t.Fatal(e)
		}
		filename, e := url.PathUnescape(filepath.Base(result.ZIP))
		if e != nil {
			t.Fatal(e)
		}
		entries := readOutputZip(t, filepath.Join(p.Temp, "export", filename))
		if len(entries) != 1 || entries[result.Name] == nil {
			t.Fatal(entries)
		}
		decoded, e := settingpack.DecodeProvider(ctx, entries[result.Name], util.SK)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(entries[result.Name], []byte(util.AESEncrypt(string(decoded)))) {
			t.Fatal("original provider encoding changed")
		}
		compressed, e := os.ReadFile(filepath.Join(p.Temp, "export", filename))
		if e != nil {
			t.Fatal(e)
		}
		request := singleUploadRequest(t, "provider.zip", compressed, nil)
		if e = request.ParseMultipartForm(0); e != nil {
			t.Fatal(e)
		}
		out, e := ImportSyncProviderRequest(ctx, request, kind)
		if e != nil || len(out) == 0 {
			t.Fatal(e)
		}
		for _, header := range request.MultipartForm.File["file"] {
			reader, e := header.Open()
			if e == nil {
				reader.Close()
				t.Fatal("provider disk scratch retained")
			}
		}
	}
	if c.Sync.WebDAV.Endpoint != "http://example.test/" || c.Sync.S3.Endpoint != "http://object.test/" {
		t.Fatal("provider normalization changed")
	}
	c.Export = conf.NewExport()
	c.Export.PandocBin = "synthetic-device-tool"
	c.AccessAuthCode = "synthetic-authority"
	c.UserData = "synthetic-account"
	result, e := ExportSettingPackageContext(ctx, "conf")
	if e != nil {
		t.Fatal(e)
	}
	filename, _ := url.PathUnescape(filepath.Base(result.ZIP))
	entries := readOutputZip(t, filepath.Join(p.Temp, "export", filename))
	raw := entries[result.Name]
	for _, private := range []string{"synthetic-device-tool", "synthetic-authority", "synthetic-account", "synthetic-password", "synthetic-secret"} {
		if bytes.Contains(raw, []byte(private)) {
			t.Fatal("portable config retained excluded field")
		}
	}
	if c.Export.PandocBin != "synthetic-device-tool" || c.AccessAuthCode != "synthetic-authority" {
		t.Fatal("export mutated owner")
	}
	before, _ := json.Marshal(c.Sync)
	if _, e = ImportSyncProviderRequest(ctx, singleUploadRequest(t, "bad.json", []byte("aa"), nil), "syncS3"); e == nil {
		t.Fatal("invalid ciphertext accepted")
	}
	after, _ := json.Marshal(c.Sync)
	if !bytes.Equal(before, after) {
		t.Fatal("bad package published settings")
	}
}

func TestRawContentActualFilesAndOwnedMultipart(t *testing.T) {
	c, _, p, _ := restoreFixture(t)
	ctx := context.Background()
	c.persistenceReadOnly = true
	address, e := ExportTempContentContext(ctx, "temporary content")
	if e != nil {
		t.Fatal(e)
	}
	parsed, e := url.Parse(address)
	if e != nil {
		t.Fatal(e)
	}
	name, _ := url.PathUnescape(strings.TrimPrefix(parsed.Path, "/export/"))
	raw, e := os.ReadFile(filepath.Join(p.Temp, "export", name))
	if e != nil || string(raw) != "temporary content" {
		t.Fatal(e)
	}
	request := singleUploadRequest(t, "same#name", []byte("first bytes"), map[string]string{"type": "text/plain"})
	if e = request.ParseMultipartForm(0); e != nil {
		t.Fatal(e)
	}
	first, e := ExportAsFileRequest(ctx, request)
	if e != nil {
		t.Fatal(e)
	}
	second, e := ExportAsFileRequest(ctx, singleUploadRequest(t, "same#name", []byte("second bytes"), map[string]string{"type": "text/plain"}))
	if e != nil || first == second {
		t.Fatal(first, second, e)
	}
	for route, want := range map[string]string{first: "first bytes", second: "second bytes"} {
		name, _ := url.PathUnescape(strings.TrimPrefix(route, "/export/"))
		raw, e = os.ReadFile(filepath.Join(p.Temp, "export", name))
		if e != nil || string(raw) != want || strings.Contains(name, "#") {
			t.Fatal(route, e)
		}
	}
	for _, header := range request.MultipartForm.File["file"] {
		reader, e := header.Open()
		if e == nil {
			reader.Close()
			t.Fatal("file export disk scratch retained")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if out, e := ExportTempContentContext(canceled, "canceled"); !errors.Is(e, context.Canceled) || out != "" {
		t.Fatal(out, e)
	}
	if out, e := ExportAsFileRequest(canceled, singleUploadRequest(t, "cancel", nil, nil)); !errors.Is(e, context.Canceled) || out != "" {
		t.Fatal(out, e)
	}
}
