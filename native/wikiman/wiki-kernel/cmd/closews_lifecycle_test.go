package cmd

import (
	"github.com/gorilla/websocket"
	"github.com/olahol/melody"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCloseWSClosesCapturedConnectionOnly(t *testing.T) {
	m := melody.New()
	defer m.Close()
	connected := make(chan *melody.Session, 3)
	m.HandleConnect(func(s *melody.Session) {
		s.Set("app", s.Request.URL.Query().Get("app"))
		s.Set("id", "same")
		connected <- s
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = m.HandleRequest(w, r) }))
	defer srv.Close()
	var sessions []*melody.Session
	for _, app := range []string{"one", "one", "two"} {
		conn, _, err := websocket.DefaultDialer.Dial("ws"+srv.URL[4:]+"/?app="+app, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		sessions = append(sessions, <-connected)
	}
	Exec(NewCommand("closews", 1, map[string]interface{}{}, sessions[0]))
	deadline := time.Now().Add(time.Second)
	for !sessions[0].IsClosed() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !sessions[0].IsClosed() {
		t.Fatal("captured connection remained open")
	}
	if sessions[1].IsClosed() || sessions[2].IsClosed() {
		t.Fatal("close crossed connection identity")
	}
}
