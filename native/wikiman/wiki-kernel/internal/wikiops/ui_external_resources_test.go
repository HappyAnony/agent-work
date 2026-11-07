package wikiops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"wiki-kernel/internal/sourcetx"
)

func TestFrameAssetsAndFormAreIndependentlyApproved(t *testing.T) {
	const css = "p { color: red }"
	const png = "fixture PNG bytes"
	var forbidden, submitted atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" {
			t.Error("frame request carried a private browser header")
		}
		switch r.URL.Path {
		case "/view", "/tampered":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<link rel="stylesheet" href="/style.css"><img src="/ok.png"><img data-ironman-verified="ok.png" src="/forbidden.png">`)
		case "/style.css":
			w.Header().Set("Content-Type", "text/css")
			fmt.Fprint(w, css)
		case "/ok.png":
			w.Header().Set("Content-Type", "image/png")
			fmt.Fprint(w, png)
		case "/send":
			submitted.Add(1)
			if r.Method != "POST" || r.ParseForm() != nil || r.PostForm.Get("answer") != "fixture" || len(r.PostForm) != 1 {
				t.Error("form submission used unexpected data")
			}
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "form accepted")
		case "/redirect":
			submitted.Add(1)
			http.Redirect(w, r, "/forbidden", http.StatusTemporaryRedirect)
		default:
			forbidden.Add(1)
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	ctx, err := WithExternalPreviewPolicy(context.Background(), []string{provider.URL}, nil, nil, []string{provider.URL}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	frame, tampered := provider.URL+"/view", provider.URL+"/tampered"
	ctx, err = WithFrameResources(ctx, map[string]map[string]string{
		frame:    {"style.css": sourcetx.Hash([]byte(css)), "ok.png": sourcetx.Hash([]byte(png))},
		tampered: {"style.css": strings.Repeat("0", 64)},
	}, map[string]FrameFormPolicy{
		frame:    {Action: provider.URL + "/send", Fields: []string{"answer"}},
		tampered: {Action: provider.URL + "/redirect", Fields: []string{"answer"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	preview := func(address string) (any, error) {
		raw, _ := json.Marshal(map[string]string{"url": address, "kind": "iframe"})
		return runUIExternalPreview(ctx, UIRequest{Data: raw})
	}
	value, err := preview(frame)
	if err != nil {
		t.Fatal(err)
	}
	out := value.(map[string]any)
	if len(out["assets"].([]frameAsset)) != 2 || strings.Count(out["text"].(string), "data-ironman-verified") != 2 {
		t.Fatal("frame snapshot has incorrect assets")
	}
	if _, err := preview(tampered); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("tampered CSS was accepted", err)
	}
	submit := func(address, mode string, fields map[string]string) (any, error) {
		raw, _ := json.Marshal(map[string]any{"url": address, "fields": fields})
		return UI(ctx, nil, UIRequest{Endpoint: "/api/asset/submitExternalFrame", Data: raw}, UIView{Mode: mode})
	}
	answer := map[string]string{"answer": "fixture"}
	if _, err := submit(frame, "docxSingle", answer); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("Single could submit a form", err)
	}
	if _, err := submit(provider.URL+"/unconfigured", "wikiDesktop", answer); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("an unconfigured preview submitted a form", err)
	}
	if _, err := submit(frame, "wikiDesktop", map[string]string{"answer": "fixture", "extra": "private"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("unapproved field accepted", err)
	}
	if submitted.Load() != 0 {
		t.Fatal("preview or rejected operation submitted a form")
	}
	if reply, err := submit(frame, "wikiDesktop", answer); err != nil || reply.(map[string]string)["text"] != "form accepted" {
		t.Fatal("approved form failed", err)
	}
	if _, err := submit(tampered, "wikiDesktop", answer); !errors.Is(err, ErrUnauthorized) || submitted.Load() != 2 || forbidden.Load() != 0 {
		t.Fatal("redirect or unapproved asset was fetched", err)
	}
}

func TestScopedPluginHotkeyPolicyRejectsConflictingOrUnmodifiedKeys(t *testing.T) {
	digest := strings.Repeat("0", 64)
	ctx, err := WithExternalPreviewPolicy(context.Background(), nil, nil, nil, nil, map[string]string{"first": digest, "second": digest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WithScopedPluginHotkeys(ctx, map[string]map[string]string{"first": {"proof": "⌥⇧⌘J"}}); err != nil {
		t.Fatal(err)
	}
	for _, profile := range []map[string]map[string]string{
		{"unknown": {"proof": "⌥⇧⌘J"}}, {"first": {"proof": "J"}},
		{"first": {"proof": "⌥⇧⌘J"}, "second": {"proof": "⌥⇧⌘J"}},
	} {
		if _, err := WithScopedPluginHotkeys(ctx, profile); !errors.Is(err, ErrArgument) {
			t.Fatal("accepted unscoped shortcut", err)
		}
	}
}
