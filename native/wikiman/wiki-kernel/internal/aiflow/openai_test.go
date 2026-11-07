package aiflow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAIActualSDKOpenAIAzureProxyAndOriginalPayload(t *testing.T) {
	for _, mode := range []string{"OpenAI", "Azure", "proxy"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || !strings.Contains(r.Header.Get("User-Agent"), "fixture-agent") {
					t.Error("method/user agent changed")
				}
				if mode == "Azure" {
					if r.Header.Get("api-key") != "synthetic" || r.URL.Query().Get("api-version") != "fixture-version" || !strings.Contains(r.URL.Path, "deployments") {
						t.Error("Azure protocol changed")
					}
				} else if r.Header.Get("Authorization") != "Bearer synthetic" {
					t.Error("OpenAI credential changed")
				}
				if mode == "proxy" && r.URL.Host != "unused.invalid" {
					t.Error("explicit proxy not used")
				}
				var body map[string]any
				if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
					t.Error(e)
				}
				msgs, ok := body["messages"].([]any)
				if !ok || len(msgs) != 3 || body["max_completion_tokens"] != float64(128) || body["temperature"] != 1.5 {
					t.Error("payload parameters changed")
				}
				for i, v := range msgs {
					m := v.(map[string]any)
					if m["role"] != "user" || m["content"] != []string{"previous", "answer", "prompt"}[i] {
						t.Error("original roles/order changed")
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":" reply "},"finish_reason":"length"},{"message":{"content":"ignored"},"finish_reason":"stop"}]}`))
			}))
			defer server.Close()
			v := settings().API
			v.APIBaseURL = server.URL + "/v1"
			v.APIModel = "fixture-model"
			v.APIMaxTokens = 128
			v.APITemperature = 1.5
			v.APIUserAgent = "fixture-agent"
			if mode == "Azure" {
				v.APIProvider = "Azure"
				v.APIVersion = "fixture-version"
			}
			if mode == "proxy" {
				v.APIBaseURL = "http://unused.invalid/v1"
				v.APIProxy = server.URL
			}
			c, e := OpenAI(v)
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			part, e := c.Chat(context.Background(), "prompt", []string{"previous", "", "answer"})
			if e != nil || part.Text != "reply" || part.Stop {
				t.Fatal(part, e)
			}
		})
	}
}
func TestAIActualSDKCancellationAndErrorsAreSanitized(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.ReadAll(r.Body); close(entered); <-release }))
	defer server.Close()
	defer close(release)
	v := settings().API
	v.APIBaseURL = server.URL
	client, e := OpenAI(v)
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := client.Chat(ctx, "fixture", nil); done <- e }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("SDK request did not enter")
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SDK cancellation lost")
	}
	v.APIProxy = "://invalid"
	if c, e := OpenAI(v); !errors.Is(e, ErrConfig) || c != nil {
		t.Fatal("bad proxy fell back to direct")
	}
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"sensitive-fixture-key","type":"fixture"}}`))
	}))
	defer errorServer.Close()
	v.APIProxy = ""
	v.APIBaseURL = errorServer.URL
	c, e := OpenAI(v)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if _, e := c.Chat(context.Background(), "fixture", nil); !errors.Is(e, ErrProvider) || strings.Contains(e.Error(), "sensitive") {
		t.Fatal(e)
	}
}
