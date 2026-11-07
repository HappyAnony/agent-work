package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/util"
)

func TestActivationContextOriginalHTTPAndUploadRevocation(t *testing.T) {
	c, file, _ := cloudBusinessFixture(t)
	before, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var mu sync.Mutex
	counts := map[string]int{}
	reject := false
	unavailable := false
	input := " \tAB\u200b-\ue000CD\n "
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cookie, e := r.Cookie("symphony")
		if e != nil || cookie.Value != "fixture-user-token" || r.Method != "POST" {
			t.Error("account cookie/method changed")
			w.WriteHeader(500)
			return
		}
		mu.Lock()
		counts[r.URL.Path]++
		denied := reject
		isUnavailable := unavailable
		mu.Unlock()
		if isUnavailable {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == "/apis/siyuan/upload/token" {
			w.Write([]byte(`{"code":0,"data":{"uploadToken":"synthetic"}}`))
			return
		}
		var body map[string]string
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil || len(body) != 1 || body["data"] != util.RemoveInvalid("AB\u200b-\ue000CD") {
			t.Error("activation normalization/body changed")
		}
		if denied {
			w.Write([]byte(`{"code":7,"msg":"fixture denied"}`))
			return
		}
		w.Write([]byte(`{"code":0,"msg":"fixture confirmation"}`))
	}))
	defer server.Close()
	s, e := c.newCloudAccount(func(ctx context.Context, region int, route string, h, b map[string]string) (cloudaccount.Response, error) {
		if region != 1 {
			t.Error("foreign region")
		}
		return requestCloudAccountURL(ctx, server.URL+route, h, b)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	get := func() {
		t.Helper()
		if _, e := LoadUploadTokenContext(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	get()
	n, msg := CheckActivationcodeContext(context.Background(), input)
	if n != 0 || msg != "fixture confirmation" {
		t.Fatal(n, msg)
	}
	get()
	if e := UseActivationcode(input); e != nil {
		t.Fatal(e)
	}
	get()
	mu.Lock()
	reject = true
	mu.Unlock()
	if n, msg := CheckActivationcode(input); n != 1 || msg != "fixture denied" {
		t.Fatal(n, msg)
	}
	if e := UseActivationcodeContext(context.Background(), input); e == nil || e.Error() != "fixture denied" {
		t.Fatal(e)
	}
	get()
	mu.Lock()
	unavailable = true
	mu.Unlock()
	if e := UseActivationcodeContext(context.Background(), input); !errors.Is(e, ErrFailedToConnectCloudServer) {
		t.Fatal("503 redemption did not retain original error", e)
	}
	after, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(before, after) || c.GetUser().UserId != "fixture-user" {
		t.Fatal("activation changed local config", e)
	}
	mu.Lock()
	defer mu.Unlock()
	if counts["/apis/siyuan/upload/token"] != 3 || counts["/apis/siyuan/checkActivationcode"] != 2 || counts["/apis/siyuan/useActivationcode"] != 3 {
		t.Fatal(counts)
	}
	recordCloudBusinessEvidence(t, "activation-http.json", map[string]any{"requests": counts, "normalizedBodyVerified": true, "accountCookieVerified": true, "confirmationMessagePreserved": true, "checkKeepsToken": true, "redemptionInvalidatesToken": true, "redemption503SentOnce": true, "configUnchanged": true, "remoteIsLoopback": true})
}

func TestActivationContextActualHTTPCancelAndOwnerClose(t *testing.T) {
	c, _, _ := cloudBusinessFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release }))
	defer server.Close()
	defer close(release)
	s, e := c.newCloudAccount(func(ctx context.Context, _ int, route string, h, b map[string]string) (cloudaccount.Response, error) {
		return requestCloudAccountURL(ctx, server.URL+route, h, b)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	done := make(chan error, 1)
	go func() { done <- UseActivationcodeContext(context.Background(), "fixture") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request never arrived")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := c.CloseCloudAccount(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("close did not join activation")
	}
}
