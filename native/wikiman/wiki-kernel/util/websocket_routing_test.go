package util

import (
	"github.com/gorilla/websocket"
	"github.com/olahol/melody"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebSocketRoutingCapturedReplyAndAppQualifiedExclusion(t *testing.T) {
	m := melody.New()
	defer m.Close()
	connected := make(chan *melody.Session, 4)
	m.HandleConnect(func(s *melody.Session) { AddPushChan(s); connected <- s })
	m.HandleDisconnect(RemovePushChan)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = m.HandleRequest(w, r) }))
	defer srv.Close()
	var clients []*websocket.Conn
	var peers []*melody.Session
	for _, query := range []string{"app=routing-a&id=same", "app=routing-b&id=same", "app=routing-a&id=other", "app=routing-a&id=same"} {
		c, _, err := websocket.DefaultDialer.Dial("ws"+srv.URL[4:]+"/?"+query, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		clients = append(clients, c)
		peers = append(peers, <-connected)
	}
	defer func() {
		for _, p := range peers {
			RemovePushChan(p)
		}
	}()
	PushEventForSession(NewCmdResult("reply", 1, PushModeSingleSelf), peers[0])
	event := NewCmdResult("others", 2, PushModeBroadcastExcludeSelf)
	event.AppId, event.SessionId = "routing-a", "same"
	PushEvent(event)
	// Per-session write queues preserve the marker after every preceding event.
	for _, peer := range peers {
		_ = peer.Write(NewCmdResult("marker", 0, PushModeSingleSelf).Bytes())
	}
	want := [][]string{{"reply", "marker"}, {"others", "marker"}, {"others", "marker"}, {"marker"}}
	for i, client := range clients {
		_ = client.SetReadDeadline(time.Now().Add(time.Second))
		for _, expected := range want[i] {
			var result Result
			if err := client.ReadJSON(&result); err != nil {
				t.Fatal(err)
			}
			if result.Cmd != expected {
				t.Fatalf("client %d: got %q want %q", i, result.Cmd, expected)
			}
		}
	}
}

func TestValidPushModeRejectsUnsupportedAndFractional(t *testing.T) {
	for _, value := range []float64{-1, 0.5, 3, 7, 1e30} {
		if ValidPushMode(value) {
			t.Fatalf("accepted unsupported mode %v", value)
		}
	}
	for _, value := range []float64{0, 1, 2, 4, 5, 6} {
		if !ValidPushMode(value) {
			t.Fatalf("rejected mode %v", value)
		}
	}
}
