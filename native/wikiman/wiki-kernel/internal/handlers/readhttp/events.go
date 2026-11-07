package readhttp

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"wiki-kernel/internal/wikiops"
)

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host == r.Host && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}

func (s *Session) events(w http.ResponseWriter, r *http.Request) {
	if !s.checkSession(r) {
		writeResult(w, nil, wikiops.ErrStaleSession)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	for key, values := range q {
		if len(values) != 1 || key != "storeId" && key != "pageId" {
			writeResult(w, nil, wikiops.ErrArgument)
			return
		}
	}
	store, page := q.Get("storeId"), q.Get("pageId")
	if !s.editorScope(r, store, page) {
		writeResult(w, nil, wikiops.ErrUnauthorized)
		return
	}
	_, err := wikiops.Pages(r.Context(), s.manager, wikiops.PageRequest{ProtocolVersion: 1, Action: "get", StoreID: store, PageID: page})
	if err != nil {
		writeResult(w, nil, err)
		return
	}
	s.mu.Lock()
	full := s.closed || len(s.connections) >= 64
	s.mu.Unlock()
	if full {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	// A new subscriber starts at the current event tail, not historical writes
	// that predate its first source read. Browser handlers await workspaceReady
	// before reading. Mutations after this baseline remain observable.
	baseline, err := s.manager.Events(r.Context(), store)
	if err != nil {
		writeResult(w, nil, err)
		return
	}
	last := uint64(0)
	epoch := ""
	seen := map[string]bool{}
	for _, event := range baseline {
		seen[event.Epoch+":"+event.OperationID+":"+event.Kind+":"+strconv.FormatUint(event.Generation, 10)] = true
		if event.Generation > last {
			last = event.Generation
		}
		epoch = event.Epoch
	}
	upgrader := websocket.Upgrader{CheckOrigin: sameOrigin, Subprotocols: []string{"wiki.v1"}, HandshakeTimeout: 5 * time.Second, ReadBufferSize: 1024, WriteBufferSize: 4096}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.mu.Lock()
	if s.closed || s.released || s.managedStore != "" && sessionForRequest(r) != s.sessionID || len(s.connections) >= 64 {
		s.mu.Unlock()
		conn.Close()
		return
	}
	view := EditorView{}
	if ticket := editorTicket(r); ticket != "" {
		var ok bool
		if view, ok = s.editors[ticket]; !ok {
			s.mu.Unlock()
			conn.Close()
			return
		}
	}
	s.connections[conn] = subscription{StoreID: store, PageID: page, EditorTicket: editorTicket(r)}
	s.wait.Add(1)
	s.mu.Unlock()
	defer func() { conn.Close(); s.mu.Lock(); delete(s.connections, conn); s.mu.Unlock(); s.wait.Done() }()
	// One bounded reader detects client close/pongs. Its owner always closes the
	// socket and joins it before returning; no workspace lease blocks mutations.
	readerDone := make(chan struct{})
	conn.SetReadLimit(1024)
	go func() {
		defer close(readerDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	defer func() { conn.Close(); <-readerDone }()
	write := func(value map[string]any) error {
		if view.ViewID != "" {
			value["viewId"] = view.ViewID
			value["sessionId"] = view.SessionID
		}
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		return conn.WriteJSON(value)
	}
	if err := write(map[string]any{"type": "workspaceReady", "protocolVersion": 1, "storeId": store, "pageId": page}); err != nil {
		return
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	ping := time.NewTicker(10 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-readerDone:
			return
		case <-r.Context().Done():
			return
		case <-ping.C:
			if err := write(map[string]any{"type": "heartbeat", "storeId": store, "pageId": page}); err != nil {
				return
			}
		case <-ticker.C:
			events, err := s.manager.Events(r.Context(), store)
			if err != nil {
				continue
			}
			if len(events) > 0 && last > 0 && (events[0].Epoch != epoch || events[0].Generation > last+1) {
				_ = write(map[string]any{"type": "resyncRequired", "storeId": store, "pageId": page})
				return
			}
			currentSeen := map[string]bool{}
			for _, event := range events {
				key := event.Epoch + ":" + event.OperationID + ":" + event.Kind + ":" + strconv.FormatUint(event.Generation, 10)
				currentSeen[key] = true
				if !seen[key] && slices.Contains(event.ChangedPageIDs, page) {
					event.ChangedPageIDs = []string{page}
					message := map[string]any{"type": "workspaceEvent", "storeId": store, "pageId": page, "event": event}
					if effect, ok := event.PageEffects[page]; ok && view.ViewID != "" {
						if view.Mode == "docxSingle" && view.EntryDocument != nil {
							visible := []string{}
							if slices.Contains(effect.Documents, view.EntryDocument.DocID) {
								visible = append(visible, view.EntryDocument.DocID)
							}
							effect.Documents = visible
							effect.Filetree = false
							effect.AttributeViews = nil
						}
						message["effect"] = effect
					}
					if err := write(message); err != nil {
						return
					}
				}
				if event.Generation > last {
					last = event.Generation
				}
				epoch = event.Epoch
			}
			seen = currentSeen
		}
	}
}
