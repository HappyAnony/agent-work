//go:build fts5

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/internal/handlers/readhttp"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func TestR03QueryCLIAndAuthenticatedHTTPShareRealReadPipeline(t *testing.T) {
	base := t.TempDir()
	roots := []string{filepath.Join(base, "CLI ' 中文"), filepath.Join(base, "HTTP ' 中文")}
	for _, root := range roots {
		if e := os.Mkdir(root, 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.CopyFS(root, os.DirFS("../../../../fixtures/shared-wiki/valid")); e != nil {
			t.Fatal(e)
		}
	}
	ctx := context.Background()
	m := workspace.NewManager(workspace.Options{ReadDefaults: true})
	defer m.Drain(ctx)
	store, e := m.OpenStore(ctx, roots[1])
	if e != nil {
		t.Fatal(e)
	}
	token := strings.Repeat("synthetic-query-token-", 3)
	session, e := readhttp.NewSession(m, token)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	const page = "1760000000000000002"
	const doc = "20261019090001-doc0001"
	const block = "20261019090002-par0001"
	cases := []struct {
		action string
		opts   any
	}{
		{"document.ast", map[string]any{"BoxID": "20261019090000-box0001", "DocID": doc}},
		{"document.info", map[string]any{"ID": doc}},
		{"block.read", map[string]any{"Mode": "block", "ID": block}},
		{"block.dom", map[string]any{"IDs": []string{block}}},
		{"block.attributes", map[string]any{"ID": doc}},
		{"block.position", map[string]any{"Mode": "breadcrumb", "ID": block}},
		{"references.backlinks", map[string]any{"Mode": "links", "ID": block}},
		{"references.query", map[string]any{"Mode": "counts", "IDs": []string{block}}},
		{"search", map[string]any{"Request": map[string]any{"Query": "knowledge", "Page": 1, "PageSize": 20}}},
		{"configuration", map[string]any{}}, {"configuration.notebooks", map[string]any{}}, {"index.rebuild", map[string]any{}},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			opts, _ := json.Marshal(tc.opts)
			q := wikiops.QueryRequest{ProtocolVersion: 1, StoreID: store, PageID: page, Action: tc.action, Options: opts}
			raw, _ := json.Marshal(q)
			var out, errs bytes.Buffer
			code := RunWithInput([]string{"query", "--root=" + roots[0], "--request=-", "--json"}, bytes.NewReader(raw), &out, &errs)
			req := httptest.NewRequest("POST", "/v1/query", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			session.ServeHTTP(rec, req)
			var a, b map[string]any
			if e := json.Unmarshal(out.Bytes(), &a); e != nil {
				t.Fatal(out.String(), e)
			}
			if e := json.Unmarshal(rec.Body.Bytes(), &b); e != nil {
				t.Fatal(rec.Body.String(), e)
			}
			if code != 0 || rec.Code != 200 || errs.Len() != 0 || a["ok"] != true {
				t.Fatal(code, rec.Code, out.String(), rec.Body.String(), errs.String())
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatal("CLI/HTTP mismatch", out.String(), rec.Body.String())
			}
		})
	}
	// Authentication runs before JSON parsing; an editor capability cannot obtain
	// the host-only query namespace even with matching page identity.
	for _, auth := range []string{"", "Bearer wrong"} {
		req := httptest.NewRequest("POST", "/v1/query", strings.NewReader("bad JSON"))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		session.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Fatal(rec.Code, rec.Body.String())
		}
	}
}

func TestR03QueryInvalidRequestsRejectBeforeOpeningRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "must-not-exist")
	inputs := []string{`{}`, `{"protocolVersion":1,"storeId":"s","pageId":"1760000000000000002","action":"sql","options":{"SQL":"DELETE FROM blocks"}}`, `{"protocolVersion":1,"storeId":"s","pageId":"1760000000000000002","action":"configuration","options":{"write":true}}`}
	for _, input := range inputs {
		var out, errs bytes.Buffer
		code := RunWithInput([]string{"query", "--root", root, "--request=-", "--json"}, strings.NewReader(input), &out, &errs)
		if code != 2 || !strings.Contains(out.String(), "invalid_argument") {
			t.Fatal(code, out.String(), errs.String())
		}
		if _, e := os.Stat(root); !os.IsNotExist(e) {
			t.Fatal("invalid query touched root", e)
		}
	}
	for _, args := range [][]string{{"query", "--root", "--help", "--request=-"}, {"query", "--root", root, "--request=-", "--output=text"}, {"query", "--root", root, "--root", root, "--request=-"}} {
		var out, errs bytes.Buffer
		if code := RunWithInput(args, strings.NewReader(`{}`), &out, &errs); code != 2 {
			t.Fatal(args, code)
		}
	}
}
