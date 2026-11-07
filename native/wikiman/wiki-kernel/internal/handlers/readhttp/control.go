package readhttp

import (
	"crypto/rand"
	"net/http"
	"wiki-kernel/internal/wikiops"
)

type ControlRequest struct {
	ProtocolVersion int    `json:"protocolVersion"`
	StoreID         string `json:"storeId"`
	Action          string `json:"action"`
	SessionID       string `json:"sessionId,omitempty"`
}
type ControlResult struct {
	StoreID   string `json:"storeId"`
	Phase     string `json:"phase"`
	SessionID string `json:"sessionId"`
}

func (s *Session) control(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		writeResult(w, nil, wikiops.ErrArgument)
		return
	}
	var request ControlRequest
	if err := wikiops.Decode(http.MaxBytesReader(w, r.Body, 4096), &request); err != nil {
		writeResult(w, nil, err)
		return
	}
	if request.ProtocolVersion != 1 || request.Action != "status" && request.Action != "release" && request.Action != "resume" {
		writeResult(w, nil, wikiops.ErrArgument)
		return
	}
	s.requests.Lock()
	defer s.requests.Unlock()
	s.mu.Lock()
	root, id, generation, closed := s.managedRoot, s.managedStore, s.sessionID, s.closed
	valid := id != "" && request.StoreID == id && !closed && (request.Action == "status" || request.SessionID == generation)
	released, complete := s.released, s.releaseComplete
	s.mu.Unlock()
	if !valid {
		writeResult(w, nil, wikiops.ErrStaleSession)
		return
	}
	result := ControlResult{StoreID: id, SessionID: generation, Phase: "ready"}
	if released {
		result.Phase = "releasing"
		if complete {
			result.Phase = "released"
		}
	}
	if request.Action == "release" && !complete {
		s.mu.Lock()
		s.released = true
		s.editors = map[string]EditorView{}
		for conn, owner := range s.connections {
			if owner.StoreID == id {
				_ = conn.Close()
			}
		}
		s.mu.Unlock()
		if err := s.manager.DrainStore(r.Context(), id); err != nil {
			result.Phase = "releasing"
			writeResult(w, result, err)
			return
		}
		result.Phase = "released"
		s.mu.Lock()
		s.releaseComplete = true
		s.mu.Unlock()
	}
	if request.Action == "resume" && released {
		if !complete {
			writeResult(w, result, wikiops.ErrStaleSession)
			return
		}
		actual, err := s.manager.OpenStore(r.Context(), root)
		if err == nil && actual != id {
			_ = s.manager.DrainStore(r.Context(), actual)
			err = wikiops.ErrStaleSession
		}
		if err == nil {
			err = s.manager.ClaimSession(r.Context(), id, false)
		}
		if err != nil {
			_ = s.manager.DrainStore(r.Context(), id)
			writeResult(w, result, err)
			return
		}
		s.mu.Lock()
		s.sessionID = rand.Text()
		s.released = false
		s.releaseComplete = false
		result.SessionID = s.sessionID
		s.mu.Unlock()
		result.Phase = "ready"
	}
	writeResult(w, result, nil)
}
