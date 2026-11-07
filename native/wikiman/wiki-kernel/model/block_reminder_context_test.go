package model

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/reminder"
)

func TestBlockReminderWorkflowFailureAndAcceptedEffects(t *testing.T) {
	c, _, _ := cloudBusinessFixture(t)
	events := []string{}
	status := 200
	s, e := c.newCloudAccountWithReminder(requestCloudAccount, requestCloudInbox, func(ctx context.Context, _ int, _ string, _ map[string]string, _ map[string]any) (cloudaccount.Response, error) {
		events = append(events, "cloud")
		return cloudaccount.Response{Status: status, Body: map[string]any{"code": float64(0)}}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	doc := &ast.Node{Type: ast.NodeDocument, ID: "root"}
	n := &ast.Node{Type: ast.NodeParagraph, ID: "leaf"}
	doc.AppendChild(n)
	tree := &parse.Tree{Root: doc}
	attrs := map[string]string{"keep": "yes"}
	stageFail := false
	var cancelAtStage context.CancelFunc
	env := &blockReminderEnvironment{gate: newAttributeMutationGate(), guard: func(ctx context.Context) error { return ctx.Err() }, wait: func(context.Context) error { events = append(events, "wait"); return nil }, load: func(context.Context, string) (*parse.Tree, map[string]string, error) {
		events = append(events, "load")
		return tree, attrs, nil
	},
		stage: func(context.Context, *parse.Tree) error {
			events = append(events, "stage")
			if stageFail {
				return errors.New("fixture stage failure")
			}
			if cancelAtStage != nil {
				cancelAtStage()
			}
			return nil
		},
		sync: func(c context.Context) error { events = append(events, "sync"); return c.Err() }, cache: func(c context.Context, id string, a map[string]string) error {
			events = append(events, "cache")
			if id != "leaf" || a["keep"] != "yes" {
				t.Error(a)
			}
			return c.Err()
		}, notify: func(c context.Context, msg string, ms int) error {
			events = append(events, "notify")
			if msg == "" || ms != 5000 {
				t.Error(msg, ms)
			}
			return c.Err()
		},
		services: reminder.Services{Location: time.UTC, Language: func(int) string { return "time %s" }, Content: func(context.Context, *ast.Node) (string, error) {
			events = append(events, "content")
			return "fixture content", nil
		}}}
	if e := setBlockReminderWithEnvironment(context.Background(), s, "leaf", "20261029120000", env); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(events, []string{"wait", "load", "content", "cloud", "stage", "sync", "cache", "notify"}) || n.IALAttr(reminder.Attribute) != "20261029120000" {
		t.Fatal(events)
	}
	if _, ok := attrs[reminder.Attribute]; ok {
		t.Fatal("input cache mutated")
	}
	for _, failure := range []string{"cloud", "stage", "date"} {
		events = nil
		status = 200
		stageFail = false
		timed := "20261030120000"
		switch failure {
		case "cloud":
			status = 503
		case "stage":
			stageFail = true
		case "date":
			timed = "bad date"
		}
		e := setBlockReminderWithEnvironment(context.Background(), s, "leaf", timed, env)
		if e == nil || n.IALAttr(reminder.Attribute) != "20261029120000" {
			t.Fatal(failure, e)
		}
		if strings.Contains(strings.Join(events, ","), "cache") || strings.Contains(strings.Join(events, ","), "notify") {
			t.Fatal("unaccepted effects", events)
		}
		if failure == "stage" && !strings.Contains(e.Error(), "remote_applied_local_not_staged") {
			t.Fatal(e)
		}
	}
	status = 200
	stageFail = false
	events = nil
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelAtStage = cancel
	if e := setBlockReminderWithEnvironment(ctx, s, "leaf", "20261031120000", env); e != nil || ctx.Err() == nil || n.IALAttr(reminder.Attribute) != "20261031120000" {
		t.Fatal("accepted effects lost on cancellation", e)
	}
	if !reflect.DeepEqual(events, []string{"wait", "load", "content", "cloud", "stage", "sync", "cache", "notify"}) {
		t.Fatal(events)
	}
	env.readOnly = true
	events = nil
	if e := setBlockReminderWithEnvironment(context.Background(), s, "leaf", "0", env); e == nil || len(events) != 0 {
		t.Fatal(e, events)
	}
}

func TestReminderContextActualHTTPNumericTimeSingleAttemptAndClose(t *testing.T) {
	c, file, _ := cloudBusinessFixture(t)
	before, _ := os.ReadFile(file)
	var mu sync.Mutex
	count := 0
	mode := "success"
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cookie, e := r.Cookie("symphony")
		if e != nil || cookie.Value != "fixture-user-token" || r.Method != "POST" || r.URL.Path != "/apis/siyuan/calendar/setBlockReminder" {
			t.Error("calendar cookie/route")
		}
		var body map[string]any
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if len(body) != 3 || body["dataId"] != "fixture" || body["data"] != "fixture content" {
			t.Error("calendar JSON")
		}
		if _, ok := body["timed"].(float64); !ok {
			t.Error("timed must be numeric")
		}
		mu.Lock()
		count++
		m := mode
		mu.Unlock()
		if m == "fail" {
			w.WriteHeader(503)
			return
		}
		if m == "block" {
			close(entered)
			<-release
			return
		}
		w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()
	defer close(release)
	s, e := c.newCloudAccountWithReminder(requestCloudAccount, requestCloudInbox, func(ctx context.Context, region int, route string, h map[string]string, b map[string]any) (cloudaccount.Response, error) {
		if region != 1 {
			t.Error("wrong region")
		}
		return requestCloudAccountJSONURL(ctx, server.URL+route, h, b)
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	for _, ms := range []int64{1760000000000, 0} {
		if e := SetCloudBlockReminderContext(context.Background(), "fixture", "fixture content", ms); e != nil {
			t.Fatal(e)
		}
	}
	mu.Lock()
	mode = "fail"
	mu.Unlock()
	if e := SetCloudBlockReminder("fixture", "fixture content", 0); !errors.Is(e, ErrFailedToConnectCloudServer) {
		t.Fatal(e)
	}
	mu.Lock()
	if count != 3 {
		t.Fatal("calendar write retried", count)
	}
	mode = "block"
	mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- SetCloudBlockReminderContext(context.Background(), "fixture", "fixture content", 0) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not arrive")
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
		t.Fatal("close did not join")
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Fatal("remote handler saved configuration")
	}
	recordCloudBusinessEvidence(t, "reminder-http.json", map[string]any{"requests": 4, "numericTimeVerified": true, "setAndCancelVerified": true, "cookieRegionAndRouteVerified": true, "http503SingleAttempt": true, "closeCancelledAndJoined": true, "configBytesUnchanged": true, "remoteIsLoopback": true})
}

func TestSubscriptionReminderOwnedDelayCancellationAndJoin(t *testing.T) {
	c, _, _ := cloudBusinessFixture(t)
	u := c.GetUser()
	u.UserSiYuanProExpireTime = float64(time.Now().Add(48 * time.Hour).UnixMilli())
	c.SetUser(u)
	entered, exited := make(chan struct{}), make(chan struct{})
	delivered := false
	e := c.scheduleSubscriptionReminder(false, func(ctx context.Context, d time.Duration) error {
		if d != 7*time.Second {
			t.Error(d)
		}
		close(entered)
		<-ctx.Done()
		close(exited)
		return ctx.Err()
	}, func(context.Context, cloudaccount.ExpirationNotice) error { delivered = true; return nil })
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("delay did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := c.CloseCloudAccount(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("delay did not join")
	}
	if delivered {
		t.Fatal("closed owner received reminder")
	}
	recordCloudBusinessEvidence(t, "reminder-owner-timer.json", map[string]any{"delayMilliseconds": 7000, "closeCancelledTimer": true, "callbackJoined": true, "staleNotificationSuppressed": true})
}
