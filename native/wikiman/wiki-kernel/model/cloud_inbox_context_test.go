package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/parse"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudaccount"
)

func modelInboxFixture(t *testing.T) (*AppConf, string) {
	t.Helper()
	c, file, _ := cloudBusinessFixture(t)
	c.Editor = conf.NewEditor()
	c.Export = conf.NewExport()
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	return c, file
}
func modelInboxReply() map[string]any {
	return map[string]any{"code": 0, "msg": "fixture", "data": map[string]any{"shorthands": []any{map[string]any{"oId": "1760000000000", "shorthandTitle": "fixture", "shorthandURL": "https://example.invalid/", "shorthandDesc": "before <audio>x</audio> after", "shorthandContent": "**owned** $a+b$\n\nfoot[^n]\n\n[^n]: note"}}, "pagination": map[string]any{"paginationPageCount": 2, "paginationRecordCount": 19}}}
}

func TestInboxContextActualHTTPProjectionAndSingleRemovalAttempt(t *testing.T) {
	c, file := modelInboxFixture(t)
	before, _ := os.ReadFile(file)
	var mu sync.Mutex
	counts := map[string]int{}
	failRemove := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cookie, e := r.Cookie("symphony")
		if e != nil || cookie.Value != "fixture-user-token" || r.Method != "POST" {
			t.Error("cookie/method changed")
			w.WriteHeader(500)
			return
		}
		mu.Lock()
		counts[r.URL.Path]++
		fail := failRemove
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "removeCloudShorthands") {
			var body map[string]any
			if e = json.NewDecoder(r.Body).Decode(&body); e != nil {
				t.Error(e)
			}
			ids, ok := body["ids"].([]any)
			if !ok || len(ids) != 3 || ids[0] != "b" || ids[1] != "a" || ids[2] != "b" || len(body) != 1 {
				t.Error("ids/order changed")
			}
			if fail {
				w.WriteHeader(503)
				return
			}
			w.Write([]byte(`{"code":0}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if len(raw) != 0 {
			t.Error("bodyless read changed")
		}
		body := modelInboxReply()
		if strings.HasSuffix(r.URL.Path, "getCloudShorthand") {
			if r.URL.Query().Get("id") != "1760000000000&other=x" || r.URL.Query().Has("other") {
				t.Error("id query escaped incorrectly")
			}
			body["data"] = body["data"].(map[string]any)["shorthands"].([]any)[0]
		} else if r.URL.Query().Get("p") != "2" {
			t.Error("page changed")
		}
		json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()
	s, e := c.newCloudAccountWithInbox(requestCloudAccount, func(ctx context.Context, region int, route string, h map[string]string, b map[string]any) (cloudaccount.Response, error) {
		if region != 1 {
			t.Error("foreign region")
		}
		var body any
		if b != nil {
			body = b
		}
		return requestCloudAccountJSONURL(ctx, server.URL+route, h, body)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	list, e := GetCloudShorthands(2)
	if e != nil {
		t.Fatal(e)
	}
	detail, e := GetCloudShorthand("1760000000000&other=x")
	if e != nil {
		t.Fatal(e)
	}
	item := list["data"].(map[string]any)["shorthands"].([]any)[0].(map[string]any)
	md := item["shorthandMd"].(string)
	// Each original parse generates fresh block IDs/timestamps; compare every
	// other preview byte while ignoring only those generated attributes.
	generated := regexp.MustCompile(` id="[0-9]{14}-[a-z0-9]{7}" updated="[0-9]{14}"`)
	for i, v := range []map[string]any{item, detail} {
		p := NewLute()
		if i == 1 {
			p.SetFootnotes(true)
		}
		p.RenderOptions.ProtyleMarkNetImg = false
		tree := parse.Parse("", []byte(md), p.ParseOptions)
		if generated.ReplaceAllString(v["shorthandContent"].(string), "") != generated.ReplaceAllString(p.ProtylePreview(tree, p.RenderOptions), "") {
			t.Fatal("original preview diverged", i)
		}
	}
	if item["shorthandDesc"] != "before  语音  after" {
		t.Fatal(item)
	}
	if e := RemoveCloudShorthands([]string{"b", "a", "b"}); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	failRemove = true
	mu.Unlock()
	if e := RemoveCloudShorthandsContext(context.Background(), []string{"b", "a", "b"}); !errors.Is(e, ErrFailedToConnectCloudServer) {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("inbox changed local configuration")
	}
	mu.Lock()
	defer mu.Unlock()
	if counts["/apis/siyuan/inbox/removeCloudShorthands"] != 2 {
		t.Fatal("removal retried", counts)
	}
	recordCloudBusinessEvidence(t, "inbox-http.json", map[string]any{"requests": counts, "accountCookieVerified": true, "bodylessReadsVerified": true, "escapedIDVerified": true, "arrayOrderVerified": true, "originalPreviewExceptGeneratedIDs": true, "remove503SentOnce": true, "configUnchanged": true, "remoteIsLoopback": true})
}

func TestInboxContextActualHTTPCancellationAndOwnerJoin(t *testing.T) {
	c, _ := modelInboxFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release }))
	defer server.Close()
	defer close(release)
	s, e := c.newCloudAccountWithInbox(requestCloudAccount, func(ctx context.Context, _ int, route string, h map[string]string, b map[string]any) (cloudaccount.Response, error) {
		return requestCloudAccountJSONURL(ctx, server.URL+route, h, nil)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	done := make(chan error, 1)
	go func() { _, e := GetCloudShorthandsContext(context.Background(), 1); done <- e }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("read never started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = c.CloseCloudAccount(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("close did not join inbox")
	}
}
