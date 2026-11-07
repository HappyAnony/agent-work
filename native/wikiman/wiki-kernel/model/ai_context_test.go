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
	"wiki-kernel/conf"
	"wiki-kernel/internal/aiflow"
	"wiki-kernel/internal/cloudaccount"
)

func aiFixture(t *testing.T) (*AppConf, string) {
	t.Helper()
	c, file, _ := cloudBusinessFixture(t)
	c.AI = conf.NewAI()
	c.Editor = conf.NewEditor()
	c.AI.OpenAI.APIKey = "synthetic"
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.CloseAI(context.Background()) })
	return c, file
}
func TestAIContextActualSDKHistorySettingsAndConfigurationNoWrite(t *testing.T) {
	c, file := aiFixture(t)
	before, _ := os.ReadFile(file)
	var mu sync.Mutex
	counts := []int{}
	models := []string{}
	missing := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Error("wrong credential")
		}
		var v map[string]any
		if json.NewDecoder(r.Body).Decode(&v) != nil {
			t.Error("invalid SDK JSON")
		}
		mu.Lock()
		counts = append(counts, len(v["messages"].([]any)))
		models = append(models, v["model"].(string))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":" fixture reply "},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	c.AI.OpenAI.APIBaseURL = server.URL + "/v1"
	c.AI.OpenAI.APIModel = "model-a"
	s, e := c.newAIService(aiflow.OpenAI, func(_ context.Context, e aiflow.Event) {
		if e == aiflow.MissingKey {
			missing++
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	c.aiOwner = s
	if text, e := ChatGPTContext(context.Background(), "one"); e != nil || text != "fixture reply" {
		t.Fatal(text, e)
	}
	if text := ChatGPT("two"); text != "fixture reply" {
		t.Fatal(text)
	}
	if text := ChatGPTWithAction(nil, "Clear context"); text != "" {
		t.Fatal(text)
	}
	ChatGPT("after clear")
	c.AI.OpenAI.APIModel = "model-b"
	ChatGPT("after model change")
	c.AI.OpenAI.APIKey = ""
	ChatGPT("disabled")
	mu.Lock()
	defer mu.Unlock()
	if len(counts) != 4 || counts[0] != 1 || counts[1] != 3 || counts[2] != 1 || counts[3] != 1 || models[3] != "model-b" || missing != 1 {
		t.Fatal(counts, models, missing)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("AI request persisted config")
	}
	recordCloudBusinessEvidence(t, "ai-sdk.json", map[string]any{"loopbackOnly": true, "requests": len(counts), "messageCounts": counts, "configIdentityResetsHistory": true, "clearActionNoProviderCall": true, "missingKeyNoProviderCall": true, "outputTrimmed": true, "configBytesUnchanged": true})
}
func TestAIContextActualHTTPOwnerCloseAndProgressCleanup(t *testing.T) {
	c, _ := aiFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	cleared := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release }))
	defer server.Close()
	defer close(release)
	c.AI.OpenAI.APIBaseURL = server.URL
	s, e := c.newAIService(aiflow.OpenAI, func(c context.Context, e aiflow.Event) {
		if e == aiflow.Clear {
			if c.Err() != nil {
				t.Error("cleanup canceled")
			}
			cleared = true
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	c.aiOwner = s
	done := make(chan error, 1)
	go func() { _, e := ChatGPTContext(context.Background(), "fixture"); done <- e }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("SDK request did not arrive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := c.CloseAI(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("owner close did not join")
	}
	if !cleared {
		t.Fatal("progress not cleared")
	}
	recordCloudBusinessEvidence(t, "ai-owner-close.json", map[string]any{"actualHTTPAborted": true, "callbackJoined": true, "progressClearedWithoutCancellation": true})
}
func TestCloudAIContextActualHTTPPayloadSingleAttemptAndCancel(t *testing.T) {
	c, file := aiFixture(t)
	before, _ := os.ReadFile(file)
	var mu sync.Mutex
	calls := 0
	mode := "length"
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, e := r.Cookie("symphony")
		if e != nil || cookie.Value != "fixture-user-token" || r.URL.Path != "/apis/siyuan/ai/chatGPT" {
			t.Error("wrong cloud route/cookie")
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		msgs := b["messages"].([]any)
		if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "user" || msgs[1].(map[string]any)["content"] != "fixture" {
			t.Error("cloud message order changed")
		}
		mu.Lock()
		calls++
		m := mode
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch m {
		case "fail":
			w.WriteHeader(503)
		case "block":
			close(entered)
			<-release
		default:
			w.Write([]byte(`{"code":0,"data":{"choices":[{"message":{"content":" raw "},"finish_reason":"length"}]}}`))
		}
	}))
	defer server.Close()
	defer close(release)
	s, e := cloudaccount.New(cloudaccount.Options{Load: func(ctx context.Context) (cloudaccount.State, error) {
		return cloudaccount.State{Region: 1, User: c.GetUser()}, ctx.Err()
	}, Commit: func(context.Context, cloudaccount.Change) error { return nil }, Request: requestCloudAccount, Decode: decodeCloudUser, Encode: func(*conf.User) (string, error) { return "", nil }, Now: time.Now, AIRequest: func(ctx context.Context, region int, route string, h map[string]string, b map[string]any) (cloudaccount.Response, error) {
		if region != 1 {
			t.Error("wrong cloud region")
		}
		return requestCloudAccountJSONURL(ctx, server.URL+route, h, b)
	}})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	text, stop, e := CloudChatGPTContext(context.Background(), "fixture", []string{"previous"})
	if e != nil || text != " raw " || stop {
		t.Fatal(text, stop, e)
	}
	mu.Lock()
	mode = "fail"
	mu.Unlock()
	if _, _, e := CloudChatGPT("fixture", []string{"previous"}); !errors.Is(e, ErrFailedToConnectCloudServer) {
		t.Fatal(e)
	}
	mu.Lock()
	if calls != 2 {
		t.Fatal("cloud AI retried", calls)
	}
	mode = "block"
	mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, _, e := CloudChatGPTContext(context.Background(), "fixture", []string{"previous"})
		done <- e
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("cloud request not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = c.CloseCloudAccount(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("cloud AI wrote configuration")
	}
	mu.Lock()
	count := calls
	mu.Unlock()
	if count != 3 {
		t.Fatal(count)
	}
	recordCloudBusinessEvidence(t, "ai-cloud.json", map[string]any{"requests": count, "originalCookieRegionAndUserRoles": true, "cloudWhitespaceAndLengthPreserved": true, "http503SingleAttempt": true, "actualCancellationJoined": true, "configBytesUnchanged": true, "loopbackOnly": true})
}
