package cloudlock

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestActualHTTPRenewalCancellationAndDetachedUnlock(t *testing.T) {
	var mu sync.Mutex
	var blob []byte
	writes, deletes := 0, 0
	entered, gone := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		switch r.Method {
		case "GET":
			w.Write(blob)
			mu.Unlock()
		case "POST":
			writes++
			if writes == 1 {
				blob = append([]byte(nil), b...)
				mu.Unlock()
				w.WriteHeader(204)
				return
			}
			mu.Unlock()
			close(entered)
			<-r.Context().Done()
			close(gone)
		case "DELETE":
			deletes++
			blob = nil
			mu.Unlock()
			w.WriteHeader(204)
		default:
			mu.Unlock()
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	client := server.Client()
	request := func(ctx context.Context, method string, b []byte) ([]byte, error) {
		r, e := http.NewRequestWithContext(ctx, method, server.URL, bytes.NewReader(b))
		if e != nil {
			return nil, e
		}
		resp, e := client.Do(r)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}
	opts := Options{Owner: "synthetic-http", RenewInterval: time.Millisecond, Read: func(ctx context.Context) ([]byte, error) { return request(ctx, "GET", nil) }, Write: func(ctx context.Context, b []byte) error { _, e := request(ctx, "POST", b); return e }, Remove: func(ctx context.Context) error { _, e := request(ctx, "DELETE", nil); return e }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- WithLock(ctx, opts, func(c context.Context) error { <-c.Done(); return c.Err() }) }()
	<-entered
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("actual IO did not join")
	}
	select {
	case <-gone:
	case <-time.After(time.Second):
		t.Fatal("server did not observe canceled renewal")
	}
	mu.Lock()
	defer mu.Unlock()
	if writes != 2 || deletes != 1 || len(blob) != 0 {
		t.Fatal(writes, deletes, len(blob))
	}
	if dir := os.Getenv("WIKI_CLOUD_BUSINESS_EVIDENCE_DIR"); dir != "" {
		os.MkdirAll(dir, 0700)
		b, _ := json.MarshalIndent(map[string]any{"writeRequests": writes, "unlockRequests": deletes, "actualRenewalHTTPCanceled": true, "detachedCleanupSucceeded": true, "joinedBeforeReturn": true}, "", "  ")
		if e := os.WriteFile(filepath.Join(dir, "cloud-lock-http.json"), append(b, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestPanicStillStopsAndReleasesLock(t *testing.T) {
	m := &memory{}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic not propagated")
			}
		}()
		WithLock(context.Background(), m.options("ours"), func(context.Context) error { panic("synthetic") })
	}()
	if m.removes != 1 || len(m.b) != 0 {
		t.Fatal("panic leaked lock")
	}
}
