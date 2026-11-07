package wikiops

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"wiki-kernel/internal/sourcetx"
)

func TestExternalPreviewOnlyFetchesApprovedOriginWithoutRedirects(t *testing.T) {
	var redirected atomic.Bool
	foreign := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Store(true)
	}))
	defer foreign.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, foreign.URL, http.StatusFound)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("preview forwarded private headers")
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<h1>offline fixture</h1>"))
	}))
	defer provider.Close()
	ctx, err := WithExternalPreviews(context.Background(), []string{provider.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	call := func(c context.Context, url, kind string) (any, error) {
		raw, _ := json.Marshal(map[string]string{"url": url, "kind": kind})
		return runUIExternalPreview(c, UIRequest{Data: raw})
	}
	if _, err := call(context.Background(), provider.URL, "iframe"); !errors.Is(err, ErrUIEndpoint) {
		t.Fatal(err)
	}
	if _, err := call(ctx, foreign.URL, "iframe"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if _, err := call(ctx, provider.URL+"/redirect", "iframe"); !errors.Is(err, fs.ErrNotExist) || redirected.Load() {
		t.Fatal("redirect escaped the approved origin", err)
	}
	if _, err := call(ctx, provider.URL, "image"); !errors.Is(err, ErrArgument) {
		t.Fatal("accepted HTML as image", err)
	}
	if value, err := call(ctx, provider.URL, "iframe"); err != nil || value.(map[string]any)["text"] != "<h1>offline fixture</h1>" {
		t.Fatal(value, err)
	}
}

func TestExternalFrameModuleSnapshotRequiresApprovedDigest(t *testing.T) {
	const script = "export const label = 'fixture';"
	var unexpected atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page/demo", "/page/tampered":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<script type="module" src="./entry.mjs"></script><script src="./extra.js"></script>`))
		case "/page/entry.mjs":
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = w.Write([]byte(script))
		default:
			unexpected.Store(true)
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	ctx, err := WithExternalPreviewPolicy(context.Background(), []string{provider.URL}, nil, nil, []string{provider.URL}, nil,
		map[string]map[string]string{
			provider.URL + "/page/demo":     {"page/entry.mjs": sourcetx.Hash([]byte(script))},
			provider.URL + "/page/tampered": {"page/entry.mjs": strings.Repeat("0", 64)},
		})
	if err != nil {
		t.Fatal(err)
	}
	preview := func(page string) (any, error) {
		raw, _ := json.Marshal(map[string]string{"url": provider.URL + page, "kind": "iframe"})
		return runUIExternalPreview(ctx, UIRequest{Data: raw})
	}
	result, err := preview("/page/demo")
	if err != nil {
		t.Fatal(err)
	}
	frame := result.(map[string]any)
	if !strings.Contains(frame["text"].(string), `src="page/entry.mjs"`) || !strings.Contains(frame["text"].(string), `src="about:blank"`) {
		t.Fatal("frame did not rewrite approved and unapproved script paths")
	}
	serve := func(origin, name string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, frame["scriptBase"].(string)+name, nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		ServeSandboxScript(ctx, w, r)
		return w
	}
	if w := serve("null", "page/entry.mjs"); w.Code != 200 || w.Body.String() != script {
		t.Fatal("approved module did not come from the snapshot")
	}
	if serve(provider.URL, "page/entry.mjs").Code != 404 || serve("null", "page/extra.js").Code != 404 || unexpected.Load() {
		t.Fatal("snapshot granted unapproved access")
	}
	if _, err := preview("/page/tampered"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("tampered module accepted", err)
	}
}
