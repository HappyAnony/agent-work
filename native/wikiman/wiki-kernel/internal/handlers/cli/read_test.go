package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wiki-kernel/internal/handlers/readhttp"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func TestReadCLIAndHTTPShareContextService(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wiki with space ' and 字")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(root, os.DirFS("../../../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	m := workspace.NewManager(workspace.Options{})
	ctx := context.Background()
	defer m.Drain(ctx)
	id, err := m.OpenStore(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	q := url.Values{"storeId": {id}, "pageId": {"1760000000000000002"}, "boxId": {"20261019090000-box0001"}, "docId": {"20261019090001-doc0001"}, "blockId": {"20261019090002-par0001"}}
	w := httptest.NewRecorder()
	readhttp.New(m).ServeHTTP(w, httptest.NewRequest("GET", "/read?"+q.Encode(), nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out, stderr bytes.Buffer
	args := []string{"read", "--root=" + root, "--store", id, "--page", q.Get("pageId"), "--box", q.Get("boxId"), "--doc", q.Get("docId"), "--block", q.Get("blockId"), "--json"}
	if code := Run(args, &out, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatal(code, out.String(), stderr.String())
	}
	var a, b struct {
		OK     bool               `json:"ok"`
		Result wikiops.ReadResult `json:"result"`
	}
	if json.Unmarshal(out.Bytes(), &a) != nil || json.Unmarshal(w.Body.Bytes(), &b) != nil || !reflect.DeepEqual(a, b) {
		t.Fatal(out.String(), w.Body.String())
	}
	for _, bad := range []string{"/read?root=" + url.QueryEscape(root), "/read?" + q.Encode() + "&pageId=other", "/read"} {
		w := httptest.NewRecorder()
		readhttp.New(m).ServeHTTP(w, httptest.NewRequest("GET", bad, nil))
		if w.Code == 200 {
			t.Fatal("accepted missing/ambiguous identity")
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".local", "index", "page.db")); !os.IsNotExist(err) {
		t.Fatal("pure read created persistent index", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".local", "locks", "source.lock")); err != nil {
		t.Fatal(err)
	}
}

func TestReadRejectsIncompleteDuplicateAndUnknownArguments(t *testing.T) {
	for _, args := range [][]string{{"read"}, {"read", "--root", "--help"}, {"read", "--root", "-h"}, {"read", "--root="}, {"read", "--root=a", "--root=b"}, {"read", "--unknown=x"}, {"read", "--doc"}} {
		var out, stderr bytes.Buffer
		args = append(args, "--json")
		if code := Run(args, &out, &stderr); code != 2 || !json.Valid(out.Bytes()) {
			t.Fatal(args, code, out.String())
		}
	}
}
