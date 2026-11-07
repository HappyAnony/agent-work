package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/announcement"
	"wiki-kernel/util"
)

func announcementFixture(t *testing.T) (*AppConf, string) {
	t.Helper()
	c, file, _ := cloudBusinessFixture(t)
	c.SetUser(nil)
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	return c, file
}
func TestAnnouncementContextActualAnonymousGETCacheLedgerAndClose(t *testing.T) {
	c, file := announcementFixture(t)
	before, _ := os.ReadFile(file)
	home := t.TempDir()
	store, _ := announcement.NewFileStore(home)
	var mu sync.Mutex
	calls, notices := 0, 0
	mode := "success"
	entered, release := make(chan struct{}), make(chan struct{})
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/apis/siyuan/version" || r.URL.Query().Get("ver") != "fixture+version" || r.Header.Get("Cookie") != "" {
			t.Error("original anonymous GET changed")
		}
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		calls++
		m := mode
		mu.Unlock()
		if m == "block" {
			close(entered)
			<-release
			return
		}
		if m == "bad" {
			w.Write([]byte(`{"announcement":123}`))
			return
		}
		w.Write([]byte(`{"version":"ignored","announcement":[{"id":"zero","title":"other region","url":"https://example.invalid/0","region":0},{"id":"one","title":"fixture","url":"https://example.invalid/1","region":1}]}`))
	}))
	defer server.Close()
	defer close(release)
	opts := &announcement.Options{Store: store, CacheTTL: 6 * time.Hour, Now: func() time.Time { return now }, Fetch: func(ctx context.Context, region int) (announcement.Response, error) {
		if region != 1 {
			t.Error("borrowed region")
		}
		return requestAnnouncementURL(ctx, server.URL+"/apis/siyuan/version?ver=fixture%2Bversion")
	}, Notify: func(ctx context.Context, v announcement.Item) error {
		if v.Id != "one" || v.Region != 1 {
			t.Error("wrong announcement")
		}
		raw, e := os.ReadFile(filepath.Join(home, ".config/siyuan/announcement.json"))
		if e != nil {
			t.Error("notice before save")
		}
		var seen []Announcement
		if json.Unmarshal(raw, &seen) != nil || len(seen) != 2 {
			t.Error("legacy ledger shape changed")
		}
		notices++
		return ctx.Err()
	}}
	s, e := c.newCloudAccountWithAnnouncements(requestCloudAccount, requestCloudInbox, requestCloudReminder, opts)
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	v, e := GetAnnouncementsContext(context.Background())
	if e != nil || len(v) != 2 {
		t.Fatal(v, e)
	}
	v[0].Title = "caller edit"
	if v := getAnnouncements(); len(v) != 2 || v[0].Title == "caller edit" {
		t.Fatal("feed cache alias")
	}
	if e := s.RunAnnouncements(context.Background(), 0); e != nil {
		t.Fatal(e)
	}
	if e := s.RunAnnouncements(context.Background(), 0); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	count := calls
	mu.Unlock()
	if count != 1 || notices != 1 {
		t.Fatal(count, notices)
	}
	mu.Lock()
	mode = "bad"
	mu.Unlock()
	now = now.Add(7 * time.Hour)
	if _, e := GetAnnouncementsContext(context.Background()); !errors.Is(e, announcement.ErrResponse) {
		t.Fatal(e)
	}
	mu.Lock()
	mode = "block"
	mu.Unlock()
	done := make(chan error, 1)
	go func() { _, e := GetAnnouncementsContext(context.Background()); done <- e }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("GET did not enter")
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
		t.Fatal("close did not join GET")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("announcement saved account config")
	}
	mu.Lock()
	count = calls
	mu.Unlock()
	if count != 3 {
		t.Fatal("unexpected GET count", count)
	}
	recordCloudBusinessEvidence(t, "announcement-http.json", map[string]any{"anonymousGETVersionAndRegionVerified": true, "requests": count, "cacheDetachedAndReused": true, "legacyLedgerContainsBothRegions": true, "matchingRegionNotices": notices, "badReplyRejected": true, "actualGETCancelledAndJoined": true, "configBytesUnchanged": true, "remoteIsLoopback": true})
}
func TestAnnouncementOwnedMinuteDelayDeduplicatesAndCloseJoins(t *testing.T) {
	c, _ := announcementFixture(t)
	home := t.TempDir()
	store, _ := announcement.NewFileStore(home)
	entered, exited := make(chan struct{}), make(chan struct{})
	fetches := 0
	opts := &announcement.Options{Store: store, CacheTTL: 6 * time.Hour, Now: time.Now, Wait: func(ctx context.Context, d time.Duration) error {
		if d != time.Minute {
			t.Error(d)
		}
		close(entered)
		<-ctx.Done()
		close(exited)
		return ctx.Err()
	}, Fetch: func(context.Context, int) (announcement.Response, error) {
		fetches++
		return announcement.Response{}, nil
	}, Notify: func(context.Context, announcement.Item) error { t.Error("closed owner notified"); return nil }}
	s, e := c.newCloudAccountWithAnnouncements(requestCloudAccount, requestCloudInbox, requestCloudReminder, opts)
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	if e = c.ScheduleAnnouncements(); e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("delay did not enter")
	}
	if e = c.ScheduleAnnouncements(); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = c.CloseCloudAccount(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("timer not joined")
	}
	if fetches != 0 {
		t.Fatal("closed delay fetched")
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("construction/delay wrote ledger")
	}
	recordCloudBusinessEvidence(t, "announcement-owner-delay.json", map[string]any{"delayMilliseconds": 60000, "pendingScheduleDeduplicated": true, "timerCancelledAndJoined": true, "fetchesAfterClose": fetches, "deviceFilesCreated": len(entries)})
}
func TestAnnouncementOriginalDeviceOptionsCaptureOwnerPathsAndDockerTTL(t *testing.T) {
	c, _ := announcementFixture(t)
	home, workspace, container := util.HomeDir, util.WorkspaceDir, util.Container
	defer func() { util.HomeDir = home; util.WorkspaceDir = workspace; util.Container = container }()
	util.HomeDir = t.TempDir()
	util.WorkspaceDir = t.TempDir()
	util.Container = util.ContainerDocker
	opts := originalAnnouncementOptions(c)
	if opts == nil || opts.CacheTTL != 24*time.Hour {
		t.Fatal("Docker TTL not inherited")
	}
	r, e := opts.Region(context.Background())
	if e != nil || r != 1 {
		t.Fatal(r, e)
	}
	util.WorkspaceDir = workspace
	if _, e := opts.Region(context.Background()); e == nil {
		t.Fatal("old owner borrowed new path")
	}
	if e := opts.Notify(context.Background(), announcement.Item{Region: 1}); e == nil {
		t.Fatal("stale callback not refused")
	}
	entries, e := os.ReadDir(util.HomeDir)
	if e != nil || len(entries) != 0 {
		t.Fatal("construction did IO")
	}
}
