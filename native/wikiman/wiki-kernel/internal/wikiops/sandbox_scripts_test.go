package wikiops

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSandboxScriptGrantScopesOriginPathAndExpiry(t *testing.T) {
	ctx, err := WithExternalPreviews(context.Background(), []string{"http://127.0.0.1:9999"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := ctx.Value(externalPreviewKey{}).(*externalPreviews)
	html, base, err := p.scripts.prepare(`<script type="module">import "./dep.mjs";</script>`,
		map[string][]byte{"dep.mjs": []byte("export const value = 1")})
	if err != nil || !strings.Contains(html, `src="__wiki_inline_0.mjs"`) {
		t.Fatal(html, base, err)
	}
	check := func(origin, resource string, status int, body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9999"+base+resource, nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		ServeSandboxScript(ctx, w, req)
		if w.Code != status || body != "" && w.Body.String() != body {
			t.Fatal(resource, origin, w.Code, w.Body.String())
		}
		if status == 200 && w.Header().Get("Access-Control-Allow-Origin") != "null" {
			t.Fatal("module response admitted a nonopaque origin")
		}
	}
	check("null", "__wiki_inline_0.mjs", 200, `import "./dep.mjs";`)
	check("null", "dep.mjs", 200, "export const value = 1")
	check("http://127.0.0.1:9999", "dep.mjs", 404, "")
	check("null", "missing.mjs", 404, "")
	check("null", "../dep.mjs", 404, "")
	key := strings.TrimSuffix(strings.TrimPrefix(base, SandboxScriptsPath), "/")
	grant := p.scripts.grants[key]
	grant.expires = time.Now().Add(-time.Second)
	p.scripts.grants[key] = grant
	check("null", "dep.mjs", 404, "")
}
