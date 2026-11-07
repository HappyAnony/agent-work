package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
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

func TestPageCLIAndAuthenticatedHTTPHaveSameDurableBehavior(t *testing.T) {
	base := t.TempDir()
	roots := []string{filepath.Join(base, "CLI ?#%& ' 中文"), filepath.Join(base, "HTTP # '& 中文")}
	for _, root := range roots {
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.CopyFS(root, os.DirFS("../../../../fixtures/shared-wiki/valid")); err != nil {
			t.Fatal(err)
		}
	}
	m := workspace.NewManager(workspace.Options{})
	defer m.Drain(context.Background())
	id, err := m.OpenStore(context.Background(), roots[1])
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("test-only", 5)
	handler, err := readhttp.NewOperations(m, token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readhttp.NewOperations(m, "short"); err == nil {
		t.Fatal("weak token accepted")
	}
	type envelope struct {
		OK              bool                 `json:"ok"`
		ProtocolVersion int                  `json:"protocolVersion"`
		Result          wikiops.PageResult   `json:"result"`
		Error           wikiops.ServiceError `json:"error"`
	}
	call := func(req wikiops.PageRequest) envelope {
		t.Helper()
		raw, _ := json.Marshal(req)
		var out, stderr bytes.Buffer
		code := RunWithInput([]string{"page", "--root=" + roots[0], "--request", "-", "--json"}, bytes.NewReader(raw), &out, &stderr)
		if stderr.Len() != 0 {
			t.Fatal(stderr.String())
		}
		r := httptest.NewRequest("POST", "/v1/pages", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		var a, b envelope
		if json.Unmarshal(out.Bytes(), &a) != nil || json.Unmarshal(w.Body.Bytes(), &b) != nil {
			t.Fatal(out.String(), w.Body.String())
		}
		if (code == 0) != a.OK || (w.Code == 200) != b.OK {
			t.Fatal(code, w.Code, out.String(), w.Body.String())
		}
		// Epoch/generation are process-local cursor identity, not source content.
		for _, v := range []*envelope{&a, &b} {
			if v.Result.Receipt != nil {
				v.Result.Receipt.Epoch = ""
				v.Result.Receipt.TreeGeneration = 0
			}
			if v.Result.Index != nil {
				v.Result.Index.Epoch = ""
				v.Result.Index.Generation = 0
			}
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("different handlers:\n%s\n%s", out.String(), w.Body.String())
		}
		return a
	}
	get := wikiops.PageRequest{ProtocolVersion: 1, Action: "get", StoreID: id, PageID: "1760000000000000002"}
	before := call(get).Result.Pages[0]
	page := before.Page
	page.Name = "Dual handler rename"
	page.Revision++
	page.UpdatedAt = "2026-10-19T02:00:00Z"
	req := wikiops.PageRequest{ProtocolVersion: 1, Action: "apply", StoreID: id, OperationID: "dual-handler-rename", DryRun: true, Updates: []wikiops.PageUpdate{{Page: page, ExpectedHash: before.Hash, EntryExpectedHash: before.EntryHash}}}
	if r := call(req); !r.OK || r.Result.Receipt.Status != "dry_run" {
		t.Fatal(r)
	}
	req.DryRun = false
	if r := call(req); !r.OK || !r.Result.Receipt.SourceCommitted {
		t.Fatal(r)
	}
	if r := call(req); !r.OK {
		t.Fatal("retry", r)
	}
	if r := call(get); r.Result.Pages[0].Page.Name != page.Name {
		t.Fatal(r)
	}
	req.OperationID = "conflicting-op"
	if r := call(req); r.OK || r.Error.Type != "source_conflict" {
		t.Fatal(r)
	}
	call(wikiops.PageRequest{ProtocolVersion: 1, Action: "index", StoreID: id, Scope: "all"})
	call(wikiops.PageRequest{ProtocolVersion: 1, Action: "list", StoreID: id, Limit: 1})
	call(wikiops.PageRequest{ProtocolVersion: 1, Action: "check", StoreID: id})
	// Persisted metadata, document bytes and derived references match, not just
	// response flags. CLI returns only after the durable source receipt.
	for _, name := range []string{"pages/1760000000000000002/page.json", "pages/1760000000000000002/data/20261019090000-box0001/20261019090001-doc0001.docj"} {
		a, err := os.ReadFile(filepath.Join(roots[0], name))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(roots[1], name))
		if err != nil || !bytes.Equal(a, b) {
			t.Fatal(name, err)
		}
	}
	var refs []workspace.Reference
	for i, root := range roots {
		manager := workspace.NewManager(workspace.Options{})
		store, err := manager.OpenStore(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		lease, err := manager.Acquire(context.Background(), store, get.PageID)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := lease.ReadReferences(context.Background(), "20261019090000-box0001", "20261019090001-doc0001")
		if err != nil {
			t.Fatal(err)
		}
		lease.Release()
		manager.Drain(context.Background())
		if i == 0 {
			refs = actual
		} else if !reflect.DeepEqual(refs, actual) {
			t.Fatal("different references")
		}
	}
	for _, auth := range []string{"", "Bearer wrong"} {
		r := httptest.NewRequest("POST", "/v1/pages", strings.NewReader(`{}`))
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatal(w.Code)
		}
	}
	for _, body := range []string{`{"protocolVersion":1,"storeId":"` + id + `","action":"list","root":"/etc"}`, `{"protocolVersion":1,"storeId":"` + id + `","action":"list","storeId":"other"}`} {
		r := httptest.NewRequest("POST", "/v1/pages", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestPageCLIMalformedArgumentsAndInputNeverOpenRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "must-not-be-created")
	for _, args := range [][]string{
		{"page", "--root", root, "--request", "--help"},
		{"page", "--root", root, "--request", "-h"},
		{"page", "--root", root, "--request="},
		{"page", "--root", root, "--request=-", "--root=duplicate"},
		{"page", "--root", root, "--request=-"},
	} {
		var out, stderr bytes.Buffer
		if code := RunWithInput(args, strings.NewReader(`{"action":"list"}`), &out, &stderr); code != 2 || !json.Valid(out.Bytes()) || stderr.Len() != 0 {
			t.Fatal(args, code, out.String(), stderr.String())
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Fatal("invalid command opened root", err)
		}
	}
}
