package announcement

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func feed() Response {
	return Response{Status: 200, Body: map[string]any{"announcement": []any{map[string]any{"id": "one", "title": "first", "url": "https://example.invalid/1", "region": float64(0)}, map[string]any{"id": "two", "title": "second", "url": "https://example.invalid/2", "region": float64(1)}, map[string]any{"id": "one", "title": "duplicate", "url": "https://example.invalid/3", "region": float64(0)}}}}
}
func options(t *testing.T, home string) (Options, *int, *int) {
	t.Helper()
	store, e := NewFileStore(home)
	if e != nil {
		t.Fatal(e)
	}
	region, calls := 0, 0
	return Options{Store: store, Region: func(c context.Context) (int, error) { return region, c.Err() }, Now: time.Now, CacheTTL: 6 * time.Hour, Fetch: func(c context.Context, r int) (Response, error) { calls++; return feed(), c.Err() }, Notify: func(context.Context, Item) error { return nil }}, &region, &calls
}
func open(t *testing.T, o Options) *Service {
	t.Helper()
	s, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := s.Close(context.Background()); e != nil {
			t.Error(e)
		}
	})
	return s
}

func TestAnnouncementOriginalMergeRegionFirstWinsAndDurableRestart(t *testing.T) {
	home := t.TempDir()
	o, region, calls := options(t, home)
	shown := []Item{}
	o.Notify = func(c context.Context, v Item) error {
		raw, e := os.ReadFile(filepath.Join(home, ledgerPath))
		if e != nil {
			t.Error(e)
		}
		var marks []Item
		if json.Unmarshal(raw, &marks) != nil || len(marks) != 2 {
			t.Error("notification before ledger commit")
		}
		shown = append(shown, v)
		return c.Err()
	}
	s := open(t, o)
	if e := s.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(shown) != 1 || shown[0].Id != "one" || shown[0].Title != "first" || *calls != 1 {
		t.Fatal(shown, *calls)
	}
	raw, _ := os.ReadFile(filepath.Join(home, ledgerPath))
	if !strings.Contains(string(raw), `"region":1`) {
		t.Fatal("other region not recorded")
	}
	*region = 1
	if e := s.Refresh(context.Background()); e != nil || len(shown) != 1 || *calls != 2 {
		t.Fatal("legacy cross-region seen rule changed", e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	s2 := open(t, o)
	if e := s2.Refresh(context.Background()); e != nil || len(shown) != 1 {
		t.Fatal("restart repeated announcement", e)
	}
	items, e := s2.List(context.Background())
	if e != nil || len(items) != 3 {
		t.Fatal(items, e)
	}
	items[0].Title = "caller mutation"
	again, e := s2.List(context.Background())
	if e != nil || again[0].Title != "first" {
		t.Fatal("cache alias escaped", e)
	}
}
func TestAnnouncementOwnedCacheTTLRegionRollbackAndMalformedFeed(t *testing.T) {
	o, region, calls := options(t, t.TempDir())
	now := time.Unix(0, 0)
	o.Now = func() time.Time { return now }
	s := open(t, o)
	get := func(want int) {
		t.Helper()
		if _, e := s.List(context.Background()); e != nil || *calls != want {
			t.Fatal(e, *calls, want)
		}
	}
	get(1)
	get(1)
	now = now.Add(6 * time.Hour)
	get(1)
	now = now.Add(time.Nanosecond)
	get(2)
	now = now.Add(-time.Hour)
	get(3)
	*region = 1
	get(4)
	for _, r := range []Response{{Status: 503}, {Status: 200, Body: map[string]any{"announcement": 123}}, {Status: 200, Body: map[string]any{"announcement": []any{nil}}}, {Status: 200, Body: map[string]any{"announcement": []any{map[string]any{"id": "bad", "title": "x", "url": "x", "region": 0.5}}}}} {
		now = now.Add(7 * time.Hour)
		s.o.Fetch = func(context.Context, int) (Response, error) { return r, nil }
		if _, e := s.List(context.Background()); e == nil || s.cached {
			t.Fatal("bad feed cached", e)
		}
	}
	s.o.Fetch = func(context.Context, int) (Response, error) {
		return Response{}, errors.New("sensitive transport error")
	}
	if e := s.Refresh(context.Background()); !errors.Is(e, ErrNetwork) {
		t.Fatal("unsanitized fetch error", e)
	}
}
func TestAnnouncementCorruptLedgerAndCommitFailuresNeverNotify(t *testing.T) {
	home := t.TempDir()
	o, _, _ := options(t, home)
	called := false
	o.Notify = func(context.Context, Item) error { called = true; return nil }
	s := open(t, o)
	for _, raw := range []string{"", `[null]`, `[{"id":""}]`, `broken`} {
		os.MkdirAll(filepath.Join(home, ledgerDir), 0700)
		os.WriteFile(filepath.Join(home, ledgerPath), []byte(raw), 0600)
		if e := s.Refresh(context.Background()); !errors.Is(e, ErrLedger) || called {
			t.Fatal(e, called)
		}
		after, _ := os.ReadFile(filepath.Join(home, ledgerPath))
		if string(after) != raw {
			t.Fatal("corrupt markers deleted or changed")
		}
	}
	s.o.Store = failedStore{}
	if e := s.Refresh(context.Background()); !errors.Is(e, os.ErrPermission) || called {
		t.Fatal(e, called)
	}
}

type failedStore struct{}

func (failedStore) Update(context.Context, func([]byte, bool) ([]byte, bool, error)) error {
	return os.ErrPermission
}
func TestAnnouncementCancelDelayFetchAndCloseJoinsCallback(t *testing.T) {
	for _, phase := range []string{"delay", "fetch", "notify"} {
		t.Run(phase, func(t *testing.T) {
			o, _, _ := options(t, t.TempDir())
			entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
			block := func(c context.Context) error { close(entered); <-release; close(exited); return c.Err() }
			delay := time.Duration(0)
			switch phase {
			case "delay":
				delay = time.Minute
				o.Wait = func(c context.Context, d time.Duration) error {
					if d != time.Minute {
						t.Error(d)
					}
					return block(c)
				}
			case "fetch":
				o.Fetch = func(c context.Context, _ int) (Response, error) { return Response{}, block(c) }
			case "notify":
				o.Notify = func(c context.Context, _ Item) error { return block(c) }
			}
			s := open(t, o)
			done := make(chan error, 1)
			go func() { done <- s.Run(context.Background(), delay) }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("callback not entered")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if e := s.Close(ctx); !errors.Is(e, context.DeadlineExceeded) {
				t.Fatal("close pretended to join ignoring callback", e)
			}
			close(release)
			if e := s.Close(context.Background()); e != nil {
				t.Fatal(e)
			}
			select {
			case <-exited:
			default:
				t.Fatal("callback not joined")
			}
			if e := <-done; !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
		})
	}
}
func TestAnnouncementConcurrentOwnersShareDeviceLedgerWithoutLostIDs(t *testing.T) {
	home := t.TempDir()
	var mu sync.Mutex
	shown := 0
	owners := []*Service{}
	for i := 0; i < 12; i++ {
		o, _, _ := options(t, home)
		o.Notify = func(context.Context, Item) error { mu.Lock(); shown++; mu.Unlock(); return nil }
		owners = append(owners, open(t, o))
	}
	var wg sync.WaitGroup
	for _, s := range owners {
		wg.Add(1)
		go func(s *Service) {
			defer wg.Done()
			if e := s.Refresh(context.Background()); e != nil {
				t.Error(e)
			}
		}(s)
	}
	wg.Wait()
	if shown != 1 {
		t.Fatal("duplicate notification", shown)
	}
	raw, e := os.ReadFile(filepath.Join(home, ledgerPath))
	if e != nil {
		t.Fatal(e)
	}
	var items []Item
	if json.Unmarshal(raw, &items) != nil || len(items) != 2 {
		t.Fatal("lost/duplicate seen IDs")
	}
}
func TestAnnouncementRegionChangeRefusesStaleCommitAndMissingFeedNoFiles(t *testing.T) {
	home := t.TempDir()
	o, region, _ := options(t, home)
	o.Fetch = func(context.Context, int) (Response, error) { *region = 1; return feed(), nil }
	s := open(t, o)
	if e := s.Refresh(context.Background()); !errors.Is(e, ErrRegion) {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(home, ledgerPath)); !os.IsNotExist(e) {
		t.Fatal("stale region wrote markers")
	}
	s.o.Fetch = func(context.Context, int) (Response, error) {
		return Response{Status: 200, Body: map[string]any{}}, nil
	}
	s.cached = false
	if e := s.Refresh(context.Background()); e != nil {
		t.Fatal(e)
	}
	entries, e := os.ReadDir(home)
	if e != nil || len(entries) != 0 {
		t.Fatal("empty feed created device files", entries, e)
	}
}
