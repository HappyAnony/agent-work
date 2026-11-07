//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gofrs/flock"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/internal/announcement"
	"wiki-kernel/internal/cloudaccount"
)

func nativeAnnouncementFeed() announcement.Response {
	return announcement.Response{Status: 200, Body: map[string]any{"announcement": []any{map[string]any{"id": "a", "title": "A", "url": "https://example.invalid/a", "region": float64(0)}, map[string]any{"id": "b", "title": "B", "url": "https://example.invalid/b", "region": float64(1)}}}}
}
func TestNativeAnnouncementOwnersShareOnlyExplicitDeviceLedger(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	home := t.TempDir()
	records := map[string]map[string]int{}
	m := open(t, root, Options{CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
		o := businessOptions(t, page)
		load := o.Load
		o.Load = func(ctx context.Context) (cloudaccount.State, error) { v, e := load(ctx); v.User = nil; return v, e }
		store, _ := announcement.NewFileStore(home)
		records[page] = map[string]int{"fetches": 0, "notices": 0}
		o.Announcements = &announcement.Options{Store: store, Now: time.Now, CacheTTL: 6 * time.Hour, Fetch: func(ctx context.Context, r int) (announcement.Response, error) {
			want := 0
			if page == pageB {
				want = 1
			}
			if r != want {
				t.Error("wrong region")
			}
			records[page]["fetches"]++
			return nativeAnnouncementFeed(), ctx.Err()
		}, Notify: func(ctx context.Context, v announcement.Item) error {
			want := 0
			if page == pageB {
				want = 1
			}
			if v.Region != want {
				t.Error("cross-region notice")
			}
			records[page]["notices"]++
			return ctx.Err()
		}}
		return o, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for _, l := range []*Lease{a, b} {
		task, e := l.StartNativeAnnouncements(context.Background(), 0)
		if e != nil {
			t.Fatal(e)
		}
		if e = task.Wait(context.Background()); e != nil {
			t.Fatal(e)
		}
	}
	if records[pageA]["notices"] != 1 || records[pageB]["notices"] != 0 {
		t.Fatal("legacy device-wide all-region seen rule changed", records)
	}
	raw, e := os.ReadFile(filepath.Join(home, ".config/siyuan/announcement.json"))
	if e != nil {
		t.Fatal(e)
	}
	var markers []announcement.Item
	if json.Unmarshal(raw, &markers) != nil || len(markers) != 2 {
		t.Fatal("bad device markers")
	}
	a.Release()
	b.Release()
	if e = m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("device task changed source or leaked")
	}
	recordCloudBusinessEvidence(t, "announcement-contexts.json", map[string]any{"owners": records, "anonymousAccountSupported": true, "explicitSharedDeviceLedger": true, "allRegionSeenRulePreserved": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks})
}
func TestNativeAnnouncementDelayFetchAndNotificationDrainWithoutSourceLock(t *testing.T) {
	uManager := open(t, fixture(t), Options{})
	u := getDocLease(t, uManager, pageA)
	if _, e := u.StartNativeAnnouncements(context.Background(), 0); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	u.Release()
	for _, phase := range []string{"delay", "fetch", "notify"} {
		t.Run(phase, func(t *testing.T) {
			root := fixture(t)
			before := digest(t, root)
			home := t.TempDir()
			entered, exited := make(chan struct{}), make(chan struct{})
			block := func(ctx context.Context) error { close(entered); <-ctx.Done(); close(exited); return ctx.Err() }
			m := open(t, root, Options{CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
				o := businessOptions(t, page)
				store, _ := announcement.NewFileStore(home)
				opts := announcement.Options{Store: store, Now: time.Now, CacheTTL: 6 * time.Hour, Fetch: func(ctx context.Context, _ int) (announcement.Response, error) {
					return nativeAnnouncementFeed(), ctx.Err()
				}, Notify: func(context.Context, announcement.Item) error { return nil }}
				switch phase {
				case "delay":
					opts.Wait = func(ctx context.Context, d time.Duration) error {
						if d != time.Minute {
							t.Error(d)
						}
						return block(ctx)
					}
				case "fetch":
					opts.Fetch = func(ctx context.Context, _ int) (announcement.Response, error) {
						return announcement.Response{}, block(ctx)
					}
				case "notify":
					opts.Notify = func(ctx context.Context, _ announcement.Item) error { return block(ctx) }
				}
				o.Announcements = &opts
				return o, nil
			}})
			l := getDocLease(t, m, pageA)
			delay := time.Duration(0)
			if phase == "delay" {
				delay = time.Minute
			}
			task, e := l.StartNativeAnnouncements(context.Background(), delay)
			if e != nil {
				t.Fatal(e)
			}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("phase not entered")
			}
			lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
			ok, e := lock.TryLock()
			if e != nil || !ok {
				t.Fatal("device task retained source lock", e)
			}
			lock.Unlock()
			l.Release()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if e = m.Drain(ctx); e != nil {
				t.Fatal(e)
			}
			select {
			case <-exited:
			default:
				t.Fatal("callback not joined")
			}
			if e = task.Wait(ctx); !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
			if digest(t, root) != before || m.Stats().Tasks != 0 {
				t.Fatal("source changed/task leaked")
			}
			_, ledgerErr := os.Stat(filepath.Join(home, ".config/siyuan/announcement.json"))
			if phase != "notify" && !os.IsNotExist(ledgerErr) {
				t.Fatal("cancel before commit marked seen")
			}
			recordCloudBusinessEvidence(t, "announcement-lifecycle-"+phase+".json", map[string]any{"phase": phase, "exclusiveSourceLockAvailable": ok, "cancelledAndJoined": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks, "markerCommittedBeforeCancel": ledgerErr == nil})
		})
	}
}
