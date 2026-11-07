//go:build fts5

package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/util"
)

func remoteDocument(t *testing.T, file string, dests ...string) string {
	t.Helper()
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var d map[string]interface{}
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	children := []interface{}{}
	for _, dest := range dests {
		children = append(children, map[string]interface{}{"Type": "NodeImage", "Children": []interface{}{map[string]string{"Type": "NodeLinkText", "Data": "asset"}, map[string]string{"Type": "NodeLinkDest", "Data": dest}}})
	}
	d["Children"].([]interface{})[0].(map[string]interface{})["Children"] = children
	raw, _ = json.Marshal(d)
	if e = os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return d["ID"].(string)
}

func TestRemoteAssetsActualHTTPFileSourceAndJoinedIndex(t *testing.T) {
	c, r, _, doc, _ := referenceModelFixture(t)
	referenceMutationPresentation(t, c)
	c.Export = conf.NewExport()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		calls.Add(1)
		if q.Header.Get("Referer") != "https://original.invalid" {
			t.Error("missing referer")
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("network bytes"))
	}))
	defer server.Close()
	local := archiveWrite(t, t.TempDir(), "local file.txt", []byte("local bytes"))
	id := remoteDocument(t, doc, server.URL+"/image.png", "file://"+url.PathEscape(local))
	c.persistenceReadOnly = true
	if e := NetAssets2LocalAssetsContext(context.Background(), id, true, "https://original.invalid"); e == nil || calls.Load() != 0 {
		t.Fatal("readonly attempted network", e)
	}
	c.persistenceReadOnly = false
	if e := NetAssets2LocalAssetsContext(context.Background(), id, true, "https://original.invalid"); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(doc)
	if e != nil || bytes.Contains(raw, []byte(server.URL)) || bytes.Contains(raw, []byte("file://")) {
		t.Fatal(string(raw), e)
	}
	entries, e := os.ReadDir(filepath.Join(util.DataDir, "assets"))
	if e != nil || len(entries) != 2 {
		t.Fatal(entries, e)
	}
	bodies := map[string]bool{}
	for _, entry := range entries {
		raw, e = os.ReadFile(filepath.Join(util.DataDir, "assets", entry.Name()))
		if e != nil {
			t.Fatal(e)
		}
		bodies[string(raw)] = true
	}
	if !bodies["network bytes"] || !bodies["local bytes"] {
		t.Fatal(bodies)
	}
	var count int
	if e = r.main.DB.QueryRow("SELECT count(*) FROM assets WHERE root_id=?", id).Scan(&count); e != nil || count != 2 {
		t.Fatal("derived index not joined", count, e)
	}
}

func TestRemoteAssetsHTTPIsCanceledAndSourceNotPublished(t *testing.T) {
	c, _, _, doc, _ := referenceModelFixture(t)
	referenceMutationPresentation(t, c)
	c.Export = conf.NewExport()
	entered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { entered <- struct{}{}; <-r.Context().Done() }))
	defer server.Close()
	id := remoteDocument(t, doc, server.URL+"/blocked.png")
	before, _ := os.ReadFile(doc)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- NetAssets2LocalAssetsContext(ctx, id, false, "") }()
	select {
	case <-entered:
	case e := <-done:
		t.Fatal("conversion failed before HTTP", e)
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP not started")
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request not joined")
	}
	after, _ := os.ReadFile(doc)
	if !bytes.Equal(before, after) {
		t.Fatal("canceled source published")
	}
}
