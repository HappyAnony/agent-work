package readhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"wiki-kernel/internal/wikiops"
)

func TestEditorCapabilitiesCannotEscapePageOrManageSource(t *testing.T) {
	m, id, root := sessionFixture(t)
	if err := m.ClaimSession(context.Background(), id, false); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("private-host-", 4)
	s, _ := NewSession(m, token)
	defer s.Close()
	generation := s.BindStore(root, id)
	server := httptest.NewServer(s)
	defer server.Close()
	call := func(route string, value any, ticket string) (int, []byte) {
		t.Helper()
		raw, _ := json.Marshal(value)
		r, _ := http.NewRequest("POST", server.URL+route, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Wiki-Session", generation)
		if ticket == "" {
			r.Header.Set("Authorization", "Bearer "+token)
		} else {
			r.Header.Set("X-Wiki-Editor", ticket)
		}
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, body
	}
	open := func(page, mode string) EditorView {
		t.Helper()
		status, raw := call("/v1/editor", editorRequest{ProtocolVersion: 1, Action: "open", StoreID: id, PageID: page, Mode: mode}, "")
		var value struct{ Result EditorView }
		_ = json.Unmarshal(raw, &value)
		if status != 200 || value.Result.Ticket == "" {
			t.Fatal(status, string(raw))
		}
		return value.Result
	}
	a, b := "1760000000000000002", "1760000000000000004"
	first := open(a, "docxSingle")
	second := open(b, "wikiDesktop")
	if first.Ticket == second.Ticket || first.SessionID != generation {
		t.Fatal("bad capability identity")
	}
	for _, request := range []wikiops.ContentRequest{
		{ProtocolVersion: 1, Action: "document.export", StoreID: id, PageID: a, BoxID: "20261019090000-box0001", DocID: "20261019100000-doc0002"},
		{ProtocolVersion: 1, Action: "document.export", StoreID: id, PageID: b, BoxID: "20261019090000-box0001", DocID: "20261019090001-doc0001"},
		{ProtocolVersion: 1, Action: "document.get", StoreID: id, PageID: a, BoxID: "20261019090000-box0001", DocID: "20261019100000-doc0002"},
		{ProtocolVersion: 1, Action: "document.list", StoreID: id, PageID: a},
	} {
		if status, _ := call("/v1/content", request, first.Ticket); status != http.StatusUnauthorized {
			t.Fatal("single view escaped entry restriction", status)
		}
	}
	if status, body := call("/v1/content", wikiops.ContentRequest{ProtocolVersion: 1, Action: "document.export", StoreID: id, PageID: a, BoxID: "20261019090000-box0001", DocID: "20261019090001-doc0001"}, first.Ticket); status != 200 || !strings.Contains(string(body), "Synthetic knowledge") {
		t.Fatal("own single export", status, string(body))
	}
	if status, _ := call("/v1/editor", editorRequest{ProtocolVersion: 1, Action: "open", StoreID: id, PageID: b, Mode: "docxSingle"}, ""); status == 200 {
		t.Fatal("single mode accepted wiki without entry")
	}
	for _, request := range []wikiops.PageRequest{
		{ProtocolVersion: 1, Action: "get", StoreID: id, PageID: b},
		{ProtocolVersion: 1, Action: "list", StoreID: id},
		{ProtocolVersion: 1, Action: "index", StoreID: id, Scope: "all"},
	} {
		if status, _ := call("/v1/pages", request, first.Ticket); status == 200 {
			t.Fatal("page escape", request)
		}
	}
	for _, route := range []string{"/v1/session", "/v1/status", "/v1/recover", "/v1/editor"} {
		if status, _ := call(route, map[string]any{"protocolVersion": 1, "storeId": id, "action": "release", "sessionId": generation}, first.Ticket); status == 200 {
			t.Fatal("editor controls host", route)
		}
	}
	for _, view := range []EditorView{first, second} {
		request := wikiops.ContentRequest{ProtocolVersion: 1, StoreID: id, PageID: view.PageID, Action: "asset.put", AssetName: "same name.bin", OperationID: "editor-asset-" + view.PageID, Data: []byte(view.PageID)}
		if status, body := call("/v1/content", request, view.Ticket); status != 200 {
			t.Fatal(status, string(body))
		}
		q := url.Values{"storeId": {id}, "pageId": {view.PageID}, "name": {request.AssetName}}
		r, _ := http.NewRequest("GET", server.URL+"/v1/resource?"+q.Encode(), nil)
		r.Header.Set("X-Wiki-Editor", view.Ticket)
		r.Header.Set("X-Wiki-Session", generation)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || string(body) != view.PageID {
			t.Fatal("asset crossed pages", response.StatusCode, string(body))
		}
		q.Set("pageId", b)
		if view.PageID == b {
			q.Set("pageId", a)
		}
		r.URL.RawQuery = q.Encode()
		response, err = http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode == 200 {
			t.Fatal("foreign resource accepted")
		}
	}
	status, raw := call("/v1/content", wikiops.ContentRequest{ProtocolVersion: 1, Action: "document.get", StoreID: id, PageID: a, BoxID: "20261019090000-box0001", DocID: "20261019090001-doc0001"}, first.Ticket)
	if status != 200 {
		t.Fatal(string(raw))
	}
	var read struct {
		Result struct {
			Hash string
			AST  json.RawMessage
		}
	}
	_ = json.Unmarshal(raw, &read)
	put := wikiops.ContentRequest{ProtocolVersion: 1, Action: "document.put", StoreID: id, PageID: a, BoxID: "20261019090000-box0001", DocID: "20261019090001-doc0001", OperationID: "editor-save-original", ExpectedHash: read.Result.Hash, AST: read.Result.AST}
	if status, body := call("/v1/content", put, first.Ticket); status != 200 {
		t.Fatal(status, string(body))
	}
	put.PageID = b
	put.OperationID = "foreign-save"
	if status, _ := call("/v1/content", put, first.Ticket); status == 200 {
		t.Fatal("foreign save accepted")
	}
	dial := func(view EditorView, page string) (*websocket.Conn, error) {
		q := url.Values{"storeId": {id}, "pageId": {page}}
		d := websocket.Dialer{Subprotocols: []string{"wiki.v1", "wiki.editor." + view.Ticket, "wiki.session." + generation}, HandshakeTimeout: time.Second}
		conn, response, err := d.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/events?"+q.Encode(), nil)
		if response != nil && err != nil {
			response.Body.Close()
		}
		return conn, err
	}
	if conn, err := dial(first, b); err == nil {
		conn.Close()
		t.Fatal("foreign subscription accepted")
	}
	conn, err := dial(first, a)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var message map[string]any
	if err = conn.ReadJSON(&message); err != nil || message["pageId"] != a || message["viewId"] != first.ViewID || message["sessionId"] != generation || conn.Subprotocol() != "wiki.v1" {
		t.Fatal(message, err)
	}
	status, raw = call("/v1/content", wikiops.ContentRequest{ProtocolVersion: 1, Action: "document.get", StoreID: id,
		PageID: a, BoxID: put.BoxID, DocID: put.DocID}, first.Ticket)
	if status != 200 || json.Unmarshal(raw, &read) != nil {
		t.Fatal("read current event preimage", status, string(raw))
	}
	put.PageID, put.OperationID = a, "editor-save-event-scope"
	put.ExpectedHash = read.Result.Hash
	put.AST = bytes.Replace(read.Result.AST, []byte("Synthetic knowledge"), []byte("Updated knowledge"), 1)
	if status, body := call("/v1/content", put, first.Ticket); status != 200 {
		t.Fatal(status, string(body))
	}
	if err = conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var scoped struct {
		Type, PageID, ViewID string
		Effect               struct {
			Documents []string
			Other     bool
		}
	}
	if err = conn.ReadJSON(&scoped); err != nil || scoped.Type != "workspaceEvent" || scoped.PageID != a ||
		scoped.ViewID != first.ViewID || scoped.Effect.Other || len(scoped.Effect.Documents) != 1 ||
		scoped.Effect.Documents[0] != put.DocID {
		t.Fatal("document event scope", scoped, err)
	}
	if status, _ := call("/v1/editor", editorRequest{ProtocolVersion: 1, Action: "close", StoreID: id, Ticket: first.Ticket}, ""); status != 200 {
		t.Fatal("close failed")
	}
	if status, _ := call("/v1/pages", wikiops.PageRequest{ProtocolVersion: 1, Action: "get", StoreID: id, PageID: a}, first.Ticket); status == 200 {
		t.Fatal("closed ticket accepted")
	}
	if status, _ := call("/v1/pages", wikiops.PageRequest{ProtocolVersion: 1, Action: "get", StoreID: id, PageID: b}, second.Ticket); status != 200 {
		t.Fatal("closing A affected B")
	}
	if status, _ := call("/v1/session", ControlRequest{ProtocolVersion: 1, StoreID: id, Action: "release", SessionID: generation}, ""); status != 200 {
		t.Fatal("release failed")
	}
	if status, _ := call("/v1/session", ControlRequest{ProtocolVersion: 1, StoreID: id, Action: "resume", SessionID: generation}, ""); status != 200 {
		t.Fatal("resume failed")
	}
	if status, _ := call("/v1/pages", wikiops.PageRequest{ProtocolVersion: 1, Action: "get", StoreID: id, PageID: b}, second.Ticket); status == 200 {
		t.Fatal("pre-release ticket accepted")
	}
}
