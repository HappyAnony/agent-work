package readhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wiki-kernel/internal/sourcetx"
)

func TestManagedSessionReleaseResumeAndStaleRequests(t *testing.T) {
	m, id, root := sessionFixture(t)
	if err := m.ClaimSession(context.Background(), id, false); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("host-control-", 4)
	session, err := NewSession(m, token)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	first := session.BindStore(root, id)
	call := func(action, generation string) (ControlResult, int) {
		t.Helper()
		raw, _ := json.Marshal(ControlRequest{ProtocolVersion: 1, StoreID: id, Action: action, SessionID: generation})
		r := httptest.NewRequest("POST", "/v1/session", bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		session.ServeHTTP(w, r)
		var value struct{ Result ControlResult }
		if json.Unmarshal(w.Body.Bytes(), &value) != nil {
			t.Fatal(w.Body.String())
		}
		return value.Result, w.Code
	}
	if result, status := call("release", first); status != 200 || result.Phase != "released" {
		t.Fatal(result, status)
	}
	if result, status := call("release", first); status != 200 || result.Phase != "released" {
		t.Fatal("release not idempotent", result, status)
	}
	source, err := sourcetx.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := source.Writer(context.Background(), false)
	if err != nil {
		t.Fatal("release retained session lock", err)
	}
	if result, status := call("resume", first); status == 200 {
		t.Fatal("resume stole CLI session", result)
	}
	writer.Close()
	resumed, status := call("resume", first)
	if status != 200 || resumed.Phase != "ready" || resumed.SessionID == first {
		t.Fatal(resumed, status)
	}
	if _, status := call("release", first); status == 200 {
		t.Fatal("old owner released a new session")
	}
	for _, nonce := range []string{"", first, resumed.SessionID} {
		r := httptest.NewRequest("POST", "/v1/pages", strings.NewReader(`{"protocolVersion":1,"action":"list","storeId":"`+id+`"}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Wiki-Session", nonce)
		w := httptest.NewRecorder()
		session.ServeHTTP(w, r)
		if (w.Code == 200) != (nonce == resumed.SessionID) {
			t.Fatal(nonce, w.Code, w.Body.String())
		}
	}
}

func TestFailedReleaseRemainsGatedAndCanBeRetried(t *testing.T) {
	m, id, root := sessionFixture(t)
	if err := m.ClaimSession(context.Background(), id, false); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("release-test-", 4)
	session, _ := NewSession(m, token)
	defer session.Close()
	nonce := session.BindStore(root, id)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- m.WithRead(context.Background(), id, func() error { close(entered); <-release; return nil })
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	control := func(ctx context.Context, action string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/session", strings.NewReader(`{"protocolVersion":1,"storeId":"`+id+`","action":"`+action+`","sessionId":"`+nonce+`"}`)).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		session.ServeHTTP(w, r)
		return w
	}
	if w := control(ctx, "release"); w.Code == 200 || !strings.Contains(w.Body.String(), `"phase":"releasing"`) {
		close(release)
		<-done
		t.Fatal(w.Code, w.Body.String())
	}
	if w := control(context.Background(), "resume"); w.Code == 200 {
		close(release)
		<-done
		t.Fatal("resumed an incompletely drained store")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if w := control(context.Background(), "release"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := control(context.Background(), "resume"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
