//go:build fts5

package workspace

import (
	"context"
	"errors"
	"github.com/gofrs/flock"
	"path/filepath"
	"testing"
	"time"
	"wiki-kernel/internal/cloudaccount"
)

func TestNativeActivationIndependentOwnersAndNoSourceMutation(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	records := map[string]map[string]any{}
	m := open(t, root, Options{CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
		o := businessOptions(t, page)
		request := o.Request
		load := o.Load
		o.Load = func(ctx context.Context) (cloudaccount.State, error) {
			v, e := load(ctx)
			v.User.UserId = "same-account-id"
			return v, e
		}
		checks, uses, tokens := 0, 0, 0
		o.Request = func(ctx context.Context, region int, route string, h, b map[string]string) (cloudaccount.Response, error) {
			if route == "/apis/siyuan/upload/token" {
				tokens++
				return request(ctx, region, route, h, b)
			}
			if b["data"] != "AB-CD" || len(b) != 1 || h["Cookie"] != "symphony=fixture-"+page {
				t.Error("foreign activation payload/identity")
			}
			want := 0
			if page == pageB {
				want = 1
			}
			if region != want {
				t.Error("foreign region")
			}
			if route == "/apis/siyuan/checkActivationcode" {
				checks++
			} else if route == "/apis/siyuan/useActivationcode" {
				uses++
			} else {
				t.Error("unexpected route")
			}
			records[page] = map[string]any{"region": region, "checks": checks, "uses": uses, "tokenFetchesBeforeNextRead": tokens}
			n := float64(0)
			msg := "fixture confirmation"
			if page == pageB && route == "/apis/siyuan/checkActivationcode" {
				n = 7
				msg = "fixture unavailable"
			}
			return cloudaccount.Response{Status: 200, Body: map[string]any{"code": n, "msg": msg}}, nil
		}
		return o, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	for i, l := range []*Lease{a, b} {
		get := func() {
			t.Helper()
			if _, e := l.NativeCloudUploadToken(context.Background()); e != nil {
				t.Fatal(e)
			}
		}
		get()
		v, e := l.NativeCloudCheckActivationcode(context.Background(), " AB\u200b-\ue000CD ")
		if e != nil || v.Code != i || v.Msg == "" || v.Data != nil {
			t.Fatal(v, e)
		}
		get()
		if e := l.NativeCloudUseActivationcode(context.Background(), " AB\u200b-\ue000CD "); e != nil {
			t.Fatal(e)
		}
		get()
		// A second check records the post-redemption fetch count without another effect.
		if _, e := l.NativeCloudCheckActivationcode(context.Background(), "AB-CD"); e != nil {
			t.Fatal(e)
		}
	}
	a.Release()
	b.Release()
	if e := m.Drain(context.Background()); e != nil {
		t.Fatal(e)
	}
	for _, v := range records {
		if v["checks"] != 2 || v["uses"] != 1 || v["tokenFetchesBeforeNextRead"] != 2 {
			t.Fatal(v)
		}
	}
	if len(records) != 2 || digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("source changed or task leaked")
	}
	recordCloudBusinessEvidence(t, "activation-contexts.json", map[string]any{"owners": records, "sameAccountIDDifferentOwner": true, "normalizedBodyVerified": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks, "rawCredentialsRecorded": false})
}

func TestNativeActivationNetworkLockReleaseDrainAndMissingBinding(t *testing.T) {
	unbound := open(t, fixture(t), Options{})
	u := getDocLease(t, unbound, pageA)
	if _, e := u.NativeCloudCheckActivationcode(context.Background(), "fixture"); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	if e := u.NativeCloudUseActivationcode(context.Background(), "fixture"); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	u.Release()
	root := fixture(t)
	before := digest(t, root)
	entered, exited := make(chan struct{}), make(chan struct{})
	m := open(t, root, Options{CloudAccountOptions: func(_, page string) (cloudaccount.Options, error) {
		o := businessOptions(t, page)
		o.Request = func(ctx context.Context, _ int, _ string, _, _ map[string]string) (cloudaccount.Response, error) {
			close(entered)
			<-ctx.Done()
			close(exited)
			return cloudaccount.Response{}, ctx.Err()
		}
		return o, nil
	}})
	l := getDocLease(t, m, pageA)
	done := make(chan error, 1)
	go func() { done <- l.NativeCloudUseActivationcode(context.Background(), "fixture") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request not started")
	}
	lock := flock.New(filepath.Join(root, ".local", "locks", "source.lock"))
	ok, e := lock.TryLock()
	if e != nil || !ok {
		t.Fatal("activation network retained source lock", e)
	}
	if e = lock.Unlock(); e != nil {
		t.Fatal(e)
	}
	l.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = m.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("drain did not join transport")
	}
	select {
	case e = <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("caller stuck")
	}
	if digest(t, root) != before || m.Stats().Tasks != 0 {
		t.Fatal("source changed or task leaked")
	}
	recordCloudBusinessEvidence(t, "activation-lifecycle.json", map[string]any{"exclusiveSourceLockAvailableDuringNetwork": ok, "drainCancelledAndJoinedTransport": true, "sourceUnchanged": true, "tasksAfterDrain": m.Stats().Tasks})
}
