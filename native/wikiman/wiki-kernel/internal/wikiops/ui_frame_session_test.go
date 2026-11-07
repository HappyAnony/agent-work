package wikiops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"wiki-kernel/internal/sourcetx"
)

func TestFrameSessionUploadsAndNestedStylesStayInScope(t *testing.T) {
	const css = `@import "./nested/theme.css"; p {background-image:url("./nested/icon.png")}`
	const theme = `h1 {color:rgb(1, 2, 3)}`
	png := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	var posts atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie := r.Header.Get("Cookie")
		if r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" {
			t.Error("request exposed browser headers")
		}
		switch r.URL.Path {
		case "/view":
			if cookie != "qa_session=bootstrap" {
				t.Error("missing private session")
			}
			http.SetCookie(w, &http.Cookie{Name: "qa_session", Value: "rotated", Path: "/"})
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<link rel="stylesheet" href="/style.css"><p>session fixture</p>`)
		case "/style.css", "/nested/theme.css", "/nested/icon.png":
			if cookie != "" {
				t.Error("session crossed into a subresource")
			}
			switch r.URL.Path {
			case "/style.css":
				w.Header().Set("Content-Type", "text/css")
				fmt.Fprint(w, css)
			case "/nested/theme.css":
				w.Header().Set("Content-Type", "text/css")
				fmt.Fprint(w, theme)
			default:
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(png)
			}
		case "/submit":
			posts.Add(1)
			if cookie != "qa_session=rotated" || r.ParseMultipartForm(1<<20) != nil ||
				r.FormValue("csrf") != "fixture-token" || r.FormValue("name") != "fixture-value" {
				t.Error("multipart request did not use the isolated rotated session and allowed fields")
				w.WriteHeader(400)
				return
			}
			file, info, err := r.FormFile("attachment")
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer file.Close()
			bytes, err := io.ReadAll(file)
			if err != nil || string(bytes) != "example" || info.Filename != "sample.txt" || info.Header.Get("Content-Type") != "text/plain" {
				t.Error("multipart file changed", err)
			}
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "uploaded")
		default:
			t.Error("unapproved request reached provider")
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	ctx, err := WithExternalPreviewPolicy(context.Background(), []string{provider.URL}, nil, nil, []string{provider.URL}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	address := provider.URL + "/view"
	ctx, err = WithFrameResources(ctx, map[string]map[string]string{address: {
		"style.css": sourcetx.Hash([]byte(css)), "nested/theme.css": sourcetx.Hash([]byte(theme)), "nested/icon.png": sourcetx.Hash(png),
	}}, map[string]FrameFormPolicy{address: {
		Action: provider.URL + "/submit", Fields: []string{"name", "csrf"}, HiddenFields: []string{"csrf"}, Files: map[string]string{"attachment": "text/plain"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = WithFrameSessions(ctx, map[string]map[string]string{address: {"qa_session": "bootstrap"}})
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]string{"kind": "iframe", "url": address})
	value, err := runUIExternalPreview(ctx, UIRequest{Data: input})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(value)
	if strings.Contains(string(raw), "qa_session") || strings.Contains(string(raw), "rotated") {
		t.Fatal("private session escaped to renderer")
	}
	var expanded string
	for _, asset := range value.(map[string]any)["assets"].([]frameAsset) {
		if asset.Name == "style.css" {
			expanded = string(asset.Data)
		}
	}
	if !strings.Contains(expanded, theme) || !strings.Contains(expanded, "data:image/png;base64,") || strings.Contains(expanded, "@import") {
		t.Fatal("approved nested CSS assets were not embedded")
	}
	submit := func(name string, data []byte) (any, error) {
		raw, _ := json.Marshal(map[string]any{"url": address, "fields": map[string]string{"name": "fixture-value", "csrf": "fixture-token"},
			"files": map[string]frameUpload{"attachment": {Name: name, Data: data}}})
		return runUIExternalFrameForm(ctx, UIRequest{Data: raw})
	}
	for _, file := range []frameUpload{{Name: "../sample.txt", Data: []byte("example")}, {Name: "sample.txt", Data: png}} {
		if _, err := submit(file.Name, file.Data); err == nil || posts.Load() != 0 {
			t.Fatal("invalid file reached provider", err)
		}
	}
	if result, err := submit("sample.txt", []byte("example")); err != nil || result.(map[string]string)["text"] != "uploaded" || posts.Load() != 1 {
		t.Fatal("approved session upload failed", err)
	}
}
