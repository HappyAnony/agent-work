package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditorAssetsConfinedAndNeverServeSourceOrLegacy(t *testing.T) {
	app, source := t.TempDir(), t.TempDir()
	for _, name := range []string{"stage/build/portable/index.html", "stage/build/portable/boot.js", "stage/protyle/js/lute/lute.min.js", "stage/protyle/js/widget/scoped.html", "appearance/langs/en_US.json", "stage/loading-pure.svg", "stage/other.svg"} {
		file := filepath.Join(app, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("synthetic static fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	assets, err := openEditorAssets(app, source)
	if err != nil {
		t.Fatal(err)
	}
	defer assets.root.Close()
	if err := os.Symlink(filepath.Join(source, "private.docj"), filepath.Join(app, "stage/build/portable/leak.js")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "private.docj"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := portableHandler(http.NotFoundHandler(), assets, "127.0.0.1:12345")
	for _, tc := range []struct {
		method, url, host string
		status            int
	}{
		{"GET", "/stage/build/portable/", "127.0.0.1:12345", 200},
		{"GET", "/stage/loading-pure.svg", "127.0.0.1:12345", 200},
		{"GET", "/stage/other.svg", "127.0.0.1:12345", 404},
		{"GET", "/stage/loading-pure.svg/../other.svg", "127.0.0.1:12345", 404},
		{"HEAD", "/stage/build/portable/boot.js", "127.0.0.1:12345", 200},
		{"GET", "/stage/build/portable/leak.js", "127.0.0.1:12345", 404},
		{"GET", "/stage/build/portable/.hidden", "127.0.0.1:12345", 404},
		{"GET", "/stage/build/portable/../desktop/index.html", "127.0.0.1:12345", 404},
		{"GET", "/appearance/", "127.0.0.1:12345", 404},
		{"GET", "/pages/private.docj", "127.0.0.1:12345", 404},
		{"GET", "/api/system/getConf", "127.0.0.1:12345", 404},
		{"GET", "/stage/build/portable/boot.js", "attacker.invalid:12345", 403},
		{"POST", "/stage/build/portable/boot.js", "127.0.0.1:12345", 405},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+tc.url, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%+v got %d: %s", tc, w.Code, w.Body.String())
		}
		if w.Code == 200 && (w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff") {
			t.Fatal("missing static protections")
		}
		if w.Code == 200 && !strings.Contains(w.Header().Get("Content-Security-Policy"), "media-src blob:;") {
			t.Fatal("media must use authenticated owned blobs, not remote source URLs")
		}
		if w.Code == 200 && !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors http://127.0.0.1:* http://localhost:* tauri://localhost dsh-app://app dsh-app://localhost;") {
			t.Fatal("editor embedding must allow only local Web and the exact application origins")
		}
	}
	widget := httptest.NewRecorder()
	handler.ServeHTTP(widget, httptest.NewRequest("GET", "http://127.0.0.1:12345/stage/protyle/js/widget/scoped.html", nil))
	policy := widget.Header().Get("Content-Security-Policy")
	if widget.Code != http.StatusOK || !strings.Contains(policy, "sandbox allow-scripts;") ||
		!strings.Contains(policy, "connect-src 'none';") || strings.Contains(policy, "allow-same-origin") ||
		!strings.Contains(policy, "http://127.0.0.1:12345/stage/protyle/js/widget/scoped.js http://127.0.0.1:12345/stage/protyle/js/widget/resources/;") {
		t.Fatal("widget frame has an unexpected policy", widget.Code, policy)
	}
	for _, nested := range []string{app, filepath.Join(app, "stage")} {
		if a, e := openEditorAssets(app, nested); e == nil {
			a.root.Close()
			t.Fatal("source overlaps app resources")
		}
	}
	for _, args := range [][]string{{"--pdf-helper"}, {"--pdf-helper", "--help"}, {"--root=a", "--store=b", "--token-file=c", "--pdf-helper=x", "--pdf-helper=y"}, {"--app-dir"}, {"--app-dir", "--help"}, {"--pandoc"}, {"--pandoc", "--help"}, {"--root=a", "--store=b", "--token-file=c", "--pandoc=x", "--pandoc=y"}, {"--root=a", "--store=b", "--token-file=c", "--app-dir=x", "--app-dir=y"}} {
		if _, e := Parse(args); e == nil {
			t.Fatal("accepted invalid app-dir", args)
		}
	}
	c, e := Parse([]string{"--root=a", "--store=b", "--token-file=c", "--app-dir=assets", "--pandoc=/trusted tool.zip"})
	if e != nil || c.AppDir != "assets" || c.Pandoc != "/trusted tool.zip" {
		t.Fatal(c, e)
	}
}
