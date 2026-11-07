package readhttp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"wiki-kernel/conf"
	"wiki-kernel/internal/sourcetx"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/workspace"
)

// Session owns every upgraded connection; Close terminates and joins them.
// HTTP listeners and managers belong to the explicit server entry, not init().
type Session struct {
	manager         *workspace.WorkspaceManager
	themes          []conf.AppearanceThemeRecord
	token           string
	mu              sync.Mutex
	connections     map[*websocket.Conn]subscription
	closed          bool
	wait            sync.WaitGroup
	requests        sync.RWMutex
	managedRoot     string
	managedStore    string
	sessionID       string
	released        bool
	releaseComplete bool
	editors         map[string]EditorView
}
type subscription struct{ StoreID, PageID, EditorTicket string }

func NewSession(manager *workspace.WorkspaceManager, token string, themes ...conf.AppearanceThemeRecord) (*Session, error) {
	if manager == nil || len(token) < 32 || len(token) > 256 || strings.ContainsAny(token, " \r\n\t,") {
		return nil, wikiops.ErrArgument
	}
	return &Session{manager: manager, themes: slices.Clone(themes), token: token, connections: map[*websocket.Conn]subscription{}, editors: map[string]EditorView{}}, nil
}
func NewOperations(manager *workspace.WorkspaceManager, token string) (http.Handler, error) {
	return NewSession(manager, token)
}
func (s *Session) Close() {
	s.mu.Lock()
	s.closed = true
	for conn := range s.connections {
		_ = conn.Close()
	}
	s.mu.Unlock()
	s.wait.Wait()
}
func writeResult(w http.ResponseWriter, result any, err error) {
	status := http.StatusOK
	if err != nil {
		status = http.StatusBadRequest
		if errors.Is(err, sourcetx.ErrConflict) || errors.Is(err, sourcetx.ErrCursor) || errors.Is(err, sourcetx.ErrBusy) {
			status = http.StatusConflict
		}
		if errors.Is(err, wikiops.ErrUnauthorized) {
			status = http.StatusUnauthorized
		}
	}
	w.WriteHeader(status)
	value := map[string]any{"ok": err == nil, "protocolVersion": 1, "result": result}
	if err != nil {
		value["error"] = wikiops.ErrorInfo(err)
	}
	_ = json.NewEncoder(w).Encode(value)
}
func (s *Session) authorized(r *http.Request) bool {
	if editorTicket(r) != "" {
		if len(r.Header.Values("Authorization")) != 0 || len(r.Header.Values("X-Wiki-Editor")) > 1 {
			return false
		}
		_, ok := s.editorFor(r)
		return ok && sameOrigin(r)
	}
	if len(r.Header.Values("Authorization")) == 1 && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.token)) == 1 {
		return true
	}
	// Browsers cannot set Authorization on WebSocket: a private subprotocol
	// carries auth in the handshake header, never a URL or echoed response.
	if r.URL.Path == "/v1/events" && len(r.Header.Values("Authorization")) == 0 {
		protocols := websocket.Subprotocols(r)
		return (len(protocols) == 2 || len(protocols) == 3) && protocols[0] == "wiki.v1" && subtle.ConstantTimeCompare([]byte(protocols[1]), []byte("wiki.auth."+s.token)) == 1
	}
	return false
}
func (s *Session) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !s.authorized(r) {
		writeResult(w, nil, wikiops.ErrUnauthorized)
		return
	}
	if r.URL.Path == "/v1/session" {
		if editorTicket(r) != "" {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		s.control(w, r)
		return
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		writeResult(w, nil, workspace.ErrClosed)
		return
	}
	if r.URL.Path == "/v1/events" {
		s.events(w, r)
		return
	}
	if r.URL.Path == "/v1/editor" {
		s.requests.Lock()
		defer s.requests.Unlock()
	} else {
		s.requests.RLock()
		defer s.requests.RUnlock()
	}
	if !s.checkSession(r) {
		writeResult(w, nil, wikiops.ErrStaleSession)
		return
	}
	if r.URL.Path == "/v1/editor" {
		s.editor(w, r)
		return
	}
	if r.URL.Path == "/v1/resource" {
		s.resource(w, r)
		return
	}
	if r.URL.RawQuery != "" {
		writeResult(w, nil, wikiops.ErrArgument)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	switch r.URL.Path {
	case "/v1/pages":
		request, err := wikiops.DecodePageRequest(r.Body)
		if err != nil {
			writeResult(w, nil, err)
			return
		}
		if !s.editorScope(r, request.StoreID, request.PageID) || editorTicket(r) != "" && request.Action != "get" {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		result, err := wikiops.Pages(r.Context(), s.manager, request)
		writeResult(w, result, err)
	case "/v1/ui":
		view, ok := s.editorFor(r)
		if !ok {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		request, err := wikiops.DecodeUIRequest(r.Body)
		if err != nil {
			writeResult(w, nil, err)
			return
		}
		if request.StoreID != view.StoreID || request.PageID != view.PageID {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		data, err := wikiops.UI(r.Context(), s.manager, request, wikiops.UIView{Mode: view.Mode, ViewID: view.ViewID, Entry: view.EntryDocument, Flashcards: view.flashcards, Themes: s.themes})
		if err != nil {
			writeResult(w, nil, err)
			return
		}
		writeResult(w, wikiops.UIResult{StoreID: view.StoreID, PageID: view.PageID, ViewID: view.ViewID, SessionID: view.SessionID, Data: data}, nil)
	case "/v1/query":
		if editorTicket(r) != "" {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		request, err := wikiops.DecodeQueryRequest(r.Body)
		if err != nil {
			writeResult(w, nil, err)
			return
		}
		if !s.editorScope(r, request.StoreID, request.PageID) {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		result, err := wikiops.Query(r.Context(), s.manager, request)
		writeResult(w, result, err)
	case "/v1/edit":
		if editorTicket(r) != "" {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		request, err := wikiops.DecodeEditRequest(r.Body)
		if err != nil {
			writeResult(w, nil, err)
			return
		}
		result, err := wikiops.Edit(r.Context(), s.manager, request)
		writeResult(w, result, err)
	case "/v1/content":
		request, err := wikiops.DecodeContentRequest(r.Body)
		if err != nil {
			writeResult(w, nil, err)
			return
		}
		if !s.editorContent(r, request) {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		result, err := wikiops.Content(r.Context(), s.manager, request)
		writeResult(w, result, err)
	case "/v1/status":
		if editorTicket(r) != "" {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		var request wikiops.StatusRequest
		if err := wikiops.Decode(r.Body, &request); err != nil {
			writeResult(w, nil, err)
			return
		}
		result, err := wikiops.Status(r.Context(), s.manager, request)
		writeResult(w, result, err)
	case "/v1/recover":
		if editorTicket(r) != "" {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		var request wikiops.RecoverRequest
		if err := wikiops.Decode(r.Body, &request); err != nil {
			writeResult(w, nil, err)
			return
		}
		result, err := wikiops.Recover(r.Context(), s.manager, request)
		writeResult(w, result, err)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// BindStore is host-only registration, never a request-supplied root.
func (s *Session) BindStore(root, id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.managedRoot = root
	s.managedStore = id
	s.sessionID = rand.Text()
	return s.sessionID
}
func (s *Session) checkSession(r *http.Request) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.managedStore == "" || !s.released && sessionForRequest(r) == s.sessionID
}
func sessionForRequest(r *http.Request) string {
	if value := r.Header.Get("X-Wiki-Session"); value != "" {
		return value
	}
	if r.URL.Path == "/v1/events" {
		protocols := websocket.Subprotocols(r)
		if len(protocols) == 3 && strings.HasPrefix(protocols[2], "wiki.session.") {
			return strings.TrimPrefix(protocols[2], "wiki.session.")
		}
	}
	return ""
}
func (s *Session) resource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	q, queryErr := url.ParseQuery(r.URL.RawQuery)
	if queryErr != nil {
		writeResult(w, nil, wikiops.ErrArgument)
		return
	}
	if !s.editorScope(r, q.Get("storeId"), q.Get("pageId")) {
		writeResult(w, nil, wikiops.ErrUnauthorized)
		return
	}
	for key, values := range q {
		if len(values) != 1 || (key != "storeId" && key != "pageId" && key != "name" && key != "boxId" && key != "docId") {
			writeResult(w, nil, wikiops.ErrArgument)
			return
		}
	}
	store, page, name := q.Get("storeId"), q.Get("pageId"), q.Get("name")
	box, doc := q.Get("boxId"), q.Get("docId")
	var result wikiops.ContentResult
	var err error
	if box != "" || doc != "" {
		if !s.editorContent(r, wikiops.ContentRequest{Action: "document.get", StoreID: store, PageID: page, BoxID: box, DocID: doc}) {
			writeResult(w, nil, wikiops.ErrUnauthorized)
			return
		}
		view, _ := s.editorFor(r)
		result, err = wikiops.DocumentAsset(r.Context(), s.manager, store, page, box, doc, name, view.Mode == "docxSingle")
	} else {
		result, err = wikiops.Content(r.Context(), s.manager, wikiops.ContentRequest{ProtocolVersion: 1, Action: "asset.get", StoreID: store, PageID: page, AssetName: name})
	}
	if err != nil {
		writeResult(w, nil, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment")
	w.Header().Set("ETag", `"`+result.Hash+`"`)
	_, _ = w.Write(result.Data)
}
