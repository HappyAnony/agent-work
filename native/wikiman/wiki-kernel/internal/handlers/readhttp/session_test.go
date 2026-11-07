package readhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

func sessionFixture(t *testing.T) (*workspace.WorkspaceManager, string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../../../fixtures/shared-wiki/valid")); err != nil {
		t.Fatal(err)
	}
	m := workspace.NewManager(workspace.Options{})
	id, err := m.OpenStore(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Drain(context.Background()) })
	return m, id, root
}
func TestScopedEventsAuthenticationResourcesAndClose(t *testing.T) {
	m, id, root := sessionFixture(t)
	if err := m.ClaimSession(context.Background(), id, false); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("test-only-token-", 3)
	session, err := NewSession(m, token)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(session)
	defer server.Close()
	defer session.Close()
	page1, page2 := "1760000000000000001", "1760000000000000002"
	dial := func(page, origin string, protocol bool) (*websocket.Conn, *http.Response, error) {
		t.Helper()
		q := url.Values{"storeId": {id}, "pageId": {page}}
		headers := http.Header{}
		d := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
		if protocol {
			d.Subprotocols = []string{"wiki.v1", "wiki.auth." + token}
		} else {
			headers.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			headers.Set("Origin", origin)
		}
		return d.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/events?"+q.Encode(), headers)
	}
	first, _, err := dial(page1, server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if first.Subprotocol() != "wiki.v1" {
		t.Fatal("auth protocol echoed", first.Subprotocol())
	}
	second, _, err := dial(page2, "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	for _, conn := range []*websocket.Conn{first, second} {
		conn.SetReadDeadline(time.Now().Add(4 * time.Second))
		var event map[string]any
		if err := conn.ReadJSON(&event); err != nil || event["type"] != "workspaceReady" {
			t.Fatal(event, err)
		}
	}
	if bad, response, err := dial(page1, "https://foreign.invalid", true); err == nil {
		bad.Close()
		t.Fatal("cross-origin WS accepted")
	} else if response == nil || response.StatusCode != 403 {
		t.Fatal(response, err)
	}
	call := func(request wikiops.ContentRequest) wikiops.ContentResult {
		t.Helper()
		raw, _ := json.Marshal(request)
		req, _ := http.NewRequest("POST", server.URL+"/v1/content", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var value struct {
			OK     bool
			Result wikiops.ContentResult
		}
		if json.NewDecoder(response.Body).Decode(&value) != nil || !value.OK {
			t.Fatal("content API failed", response.StatusCode)
		}
		return value.Result
	}
	request := wikiops.ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: page1, Action: "asset.put", AssetName: "events image.bin", OperationID: "scoped-event-asset", Data: []byte{1, 2, 3}}
	result := call(request)
	if result.Receipt == nil || !result.Receipt.SourceCommitted {
		t.Fatal(result)
	}
	var event struct {
		Type, StoreID, PageID string
		Event                 sourcetx.Event
	}
	if err := first.ReadJSON(&event); err != nil || event.Type != "workspaceEvent" || event.StoreID != id || event.PageID != page1 || len(event.Event.ChangedPageIDs) != 1 || event.Event.ChangedPageIDs[0] != page1 {
		t.Fatal(event, err)
	}
	late, _, err := dial(page1, server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	defer late.Close()
	late.SetReadDeadline(time.Now().Add(4 * time.Second))
	var ready map[string]any
	if err = late.ReadJSON(&ready); err != nil || ready["type"] != "workspaceReady" {
		t.Fatal(ready, err)
	}
	next := request
	next.OperationID = "after-late-subscribe"
	next.AssetName = "next image.bin"
	call(next)
	if err = late.ReadJSON(&event); err != nil || event.Event.OperationID != next.OperationID {
		t.Fatal("new subscriber replayed history or missed next mutation", event, err)
	}
	second.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if err := second.ReadJSON(&event); err == nil {
		t.Fatal("another workspace's event leaked", event)
	}
	q := url.Values{"storeId": {id}, "pageId": {page1}, "name": {request.AssetName}}
	resource, _ := http.NewRequest("GET", server.URL+"/v1/resource?"+q.Encode(), nil)
	resource.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(resource)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || response.Header.Get("Content-Disposition") != "attachment" || response.Header.Get("ETag") == "" {
		t.Fatal(response.StatusCode, response.Header)
	}
	var b bytes.Buffer
	b.ReadFrom(response.Body)
	response.Body.Close()
	if !bytes.Equal(b.Bytes(), request.Data) {
		t.Fatal(b.Bytes())
	}
	for _, suffix := range []string{"/v1/resource?token=" + token, "/v1/events?storeId=" + id} {
		req, _ := http.NewRequest("GET", server.URL+suffix, nil)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatal("unauthenticated access", response.StatusCode)
		}
	}
	// CLI/new writer cannot steal the persistent server editor session.
	source, err := sourcetx.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if writer, err := source.Writer(context.Background(), false); err == nil {
		writer.Close()
		t.Fatal("writer stole server session")
	}
	done := make(chan struct{})
	go func() { session.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("session failed to join websocket tasks")
	}
}
