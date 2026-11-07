//go:build fts5

package workspace

import (
	"context"
	"errors"
	"github.com/gofrs/flock"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/reminder"
)

func TestNativeReminderActualOwnerCandidatesCloudAndNoSourceWrite(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	records := map[string]map[string]any{}
	m := open(t, root, Options{Configuration: inboxConfiguration, CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
		o := businessOptions(t, page)
		calls := 0
		o.ReminderRequest = func(c context.Context, region int, route string, h map[string]string, b map[string]any) (cloudaccount.Response, error) {
			calls++
			want := 0
			if page == pageB {
				want = 1
			}
			if region != want || h["Cookie"] != "symphony=fixture-"+page || route != "/apis/siyuan/calendar/setBlockReminder" || b["dataId"] != blockID {
				t.Error("foreign reminder identity")
			}
			if _, ok := b["timed"].(int64); !ok {
				t.Error("numeric timed lost")
			}
			records[page] = map[string]any{"calls": calls, "region": region}
			return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0)}}, nil
		}
		return o, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	contents := map[string]string{}
	for _, l := range []*Lease{a, b} {
		p, e := l.PlanNativeBlockReminder(context.Background(), blockID, "20261029120000")
		if e != nil {
			t.Fatal(e)
		}
		if p.TargetID != blockID || p.Payload.ID != blockID || len(p.SourceHashes) != 1 || p.Node.IALAttr(reminder.Attribute) != "" {
			t.Fatal("not a detached preparation")
		}
		page := pageA
		if l == b {
			page = pageB
		}
		contents[page] = p.Payload.Content
		if e := l.NativeCloudSetBlockReminder(context.Background(), p.Payload); e != nil {
			t.Fatal(e)
		}
		p.Node.SetIALAttr(reminder.Attribute, "candidate-only")
		again, e := l.PlanNativeBlockReminder(context.Background(), blockID, "0")
		if e != nil || again.Node.IALAttr(reminder.Attribute) != "" || again.Payload.Milliseconds != 0 {
			t.Fatal("candidate mutated owner/source", e)
		}
		if e := l.NativeCloudSetBlockReminder(context.Background(), again.Payload); e != nil {
			t.Fatal(e)
		}
	}
	if contents[pageA] == contents[pageB] {
		t.Fatal("same-ID cross-page source leak")
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("remote call wrote local source")
	}
	recordCloudBusinessEvidence(t, "reminder-contexts.json", map[string]any{"owners": records, "differentPageContentVerified": true, "detachedSourceCandidateVerified": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks, "rawContentAndCredentialsRecorded": false})
}
func TestNativeReminderCloudAndSubscriptionTimerDrainTogether(t *testing.T) {
	uManager := open(t, fixture(t), Options{})
	u := getDocLease(t, uManager, pageA)
	if e := u.NativeCloudSetBlockReminder(context.Background(), reminder.Payload{}); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	u.Release()
	root := fixture(t)
	before := digest(t, root)
	entered, exited, planned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	m := open(t, root, Options{Configuration: inboxConfiguration, CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
		o := businessOptions(t, page)
		load := o.Load
		o.Load = func(c context.Context) (cloudaccount.State, error) {
			v, e := load(c)
			v.User.UserSiYuanProExpireTime = float64(time.Now().Add(48 * time.Hour).UnixMilli())
			once.Do(func() { close(planned) })
			return v, e
		}
		o.ReminderRequest = func(c context.Context, _ int, _ string, _ map[string]string, _ map[string]any) (cloudaccount.Response, error) {
			close(entered)
			<-c.Done()
			close(exited)
			return cloudaccount.Response{}, c.Err()
		}
		return o, nil
	}})
	l := getDocLease(t, m, pageA)
	timer, e := l.StartNativeSubscriptionReminder(context.Background(), false, func(context.Context, cloudaccount.ExpirationNotice) error {
		t.Error("drained timer notified")
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-planned:
	case <-time.After(3 * time.Second):
		t.Fatal("timer did not plan")
	}
	done := make(chan error, 1)
	go func() { done <- l.NativeCloudSetBlockReminder(context.Background(), reminder.Payload{ID: blockID}) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("cloud request did not enter")
	}
	lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
	ok, e := lock.TryLock()
	if e != nil || !ok {
		t.Fatal("network retained source lock", e)
	}
	lock.Unlock()
	l.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e := m.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("cloud callback not joined")
	}
	if e := timer.Wait(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal("timer did not cancel", e)
	}
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("cloud request stuck")
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("source changed or tasks leaked")
	}
	recordCloudBusinessEvidence(t, "reminder-lifecycle.json", map[string]any{"exclusiveSourceLockDuringNetwork": ok, "cloudCancelledAndJoined": true, "subscriptionTimerCancelledAndJoined": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks})
}
