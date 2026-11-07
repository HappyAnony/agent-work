package readhttp

import (
	"crypto/rand"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"wiki-kernel/internal/wikiops"
	"wiki-kernel/internal/wikistore"
)

// An editor capability is scoped to ONE source generation/page/view. The host
// Bearer credential is never needed by browser editors, nor put in URLs.
type EditorView struct {
	StoreID       string                   `json:"storeId"`
	PageID        string                   `json:"pageId"`
	SessionID     string                   `json:"sessionId"`
	Mode          string                   `json:"mode"`
	ViewID        string                   `json:"viewId"`
	Ticket        string                   `json:"ticket"`
	EntryDocument *wikistore.EntryDocument `json:"entryDocument,omitempty"`
	flashcards    *wikiops.UIFlashcardState
}

func (s *Session) editorContent(r *http.Request, request wikiops.ContentRequest) bool {
	if editorTicket(r) == "" {
		return true
	}
	view, ok := s.editorFor(r)
	if !ok || view.StoreID != request.StoreID || view.PageID != request.PageID || request.PageUpdate != nil || request.NotebookName != "" {
		return false
	}
	switch request.Action {
	case "document.create", "document.import", "document.rename", "document.move", "document.sort", "document.delete", "document.deleted", "document.recover",
		"document.version.save", "document.version.list", "document.version.get", "document.version.apply", "asset.rename":
		return false
	}
	if view.Mode != "docxSingle" {
		return true
	}
	if strings.HasPrefix(request.Action, "asset.") {
		return true
	}
	entry := view.EntryDocument
	return entry != nil && request.BoxID == entry.BoxID && request.DocID == entry.DocID && (strings.HasPrefix(request.Action, "block.") || request.Action == "document.get" || request.Action == "document.put" || request.Action == "document.restore" || request.Action == "document.render" || request.Action == "document.export" || request.Action == "document.transactions" || request.Action == "document.history") && (request.TargetDocID == "" || request.TargetDocID == entry.DocID) && (request.ParentDocID == "" || request.ParentDocID == entry.DocID)
}

type editorRequest struct {
	ProtocolVersion int    `json:"protocolVersion"`
	Action          string `json:"action"`
	StoreID         string `json:"storeId"`
	PageID          string `json:"pageId,omitempty"`
	Mode            string `json:"mode,omitempty"`
	Ticket          string `json:"ticket,omitempty"`
}

func editorTicket(r *http.Request) string {
	if values := r.Header.Values("X-Wiki-Editor"); len(values) == 1 {
		return values[0]
	}
	if r.URL.Path == "/v1/events" {
		protocols := websocket.Subprotocols(r)
		if len(protocols) == 3 && protocols[0] == "wiki.v1" && strings.HasPrefix(protocols[1], "wiki.editor.") {
			return strings.TrimPrefix(protocols[1], "wiki.editor.")
		}
	}
	return ""
}
func (s *Session) editorFor(r *http.Request) (EditorView, bool) {
	ticket := editorTicket(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	view, ok := s.editors[ticket]
	return view, ok && ticket != "" && !s.closed && !s.released && view.SessionID == s.sessionID
}
func (s *Session) editorScope(r *http.Request, store, page string) bool {
	if editorTicket(r) == "" {
		return true
	}
	view, ok := s.editorFor(r)
	return ok && view.StoreID == store && view.PageID == page
}
func (s *Session) editor(w http.ResponseWriter, r *http.Request) {
	if editorTicket(r) != "" || r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeResult(w, nil, wikiops.ErrArgument)
		return
	}
	var request editorRequest
	if err := wikiops.Decode(http.MaxBytesReader(w, r.Body, 4096), &request); err != nil {
		writeResult(w, nil, err)
		return
	}
	if request.ProtocolVersion != 1 || request.StoreID == "" {
		writeResult(w, nil, wikiops.ErrArgument)
		return
	}
	if request.Action == "close" {
		if request.Ticket == "" || request.PageID != "" || request.Mode != "" {
			writeResult(w, nil, wikiops.ErrArgument)
			return
		}
		s.mu.Lock()
		view, ok := s.editors[request.Ticket]
		if ok && view.StoreID == request.StoreID {
			delete(s.editors, request.Ticket)
			for conn, sub := range s.connections {
				if sub.EditorTicket == request.Ticket {
					_ = conn.Close()
				}
			}
		}
		s.mu.Unlock()
		if !ok || view.StoreID != request.StoreID {
			writeResult(w, nil, wikiops.ErrStaleSession)
			return
		}
		writeResult(w, map[string]bool{"closed": true}, nil)
		return
	}
	if request.Action != "open" || request.Ticket != "" || request.Mode != "wikiDesktop" && request.Mode != "docxSingle" {
		writeResult(w, nil, wikiops.ErrArgument)
		return
	}
	result, err := wikiops.Pages(r.Context(), s.manager, wikiops.PageRequest{ProtocolVersion: 1, Action: "get", StoreID: request.StoreID, PageID: request.PageID})
	if err != nil {
		writeResult(w, nil, err)
		return
	}
	if len(result.Pages) != 1 || result.Pages[0].Page.DeletedAt != nil || result.Pages[0].Page.Type == "table" || request.Mode == "docxSingle" && result.Pages[0].Page.Type != "docx" {
		writeResult(w, nil, wikiops.ErrArgument)
		return
	}
	s.mu.Lock()
	if s.closed || s.released || s.managedStore != request.StoreID || len(s.editors) >= 64 {
		s.mu.Unlock()
		writeResult(w, nil, wikiops.ErrStaleSession)
		return
	}
	view := EditorView{StoreID: request.StoreID, PageID: request.PageID, SessionID: s.sessionID, Mode: request.Mode, ViewID: rand.Text(), Ticket: rand.Text(), EntryDocument: result.Pages[0].Page.EntryDocument}
	if view.Mode == "wikiDesktop" {
		view.flashcards = &wikiops.UIFlashcardState{}
	}
	s.editors[view.Ticket] = view
	s.mu.Unlock()
	writeResult(w, view, nil)
}
