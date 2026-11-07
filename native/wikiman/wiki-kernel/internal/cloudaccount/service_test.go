// SPDX-License-Identifier: AGPL-3.0-or-later
package cloudaccount

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func fixture(t *testing.T) (*State, *Options) {
	t.Helper()
	state := &State{}
	o := &Options{Now: func() time.Time { return time.Unix(2000000000, 0) }}
	o.Load = func(context.Context) (State, error) { return *state, nil }
	o.Commit = func(ctx context.Context, p Change) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if p.Region != nil {
			state.Region = *p.Region
		}
		if p.SetUser {
			state.Cipher = p.Cipher
			state.User, _ = clone(p.User)
		}
		return nil
	}
	o.Decode = func(s string, _ bool) (u *conf.User, e error) { e = json.Unmarshal([]byte(s), &u); return }
	o.Encode = func(u *conf.User) (string, error) { b, e := json.Marshal(u); return string(b), e }
	o.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		return Response{Status: 200, Body: map[string]any{"code": float64(0), "msg": "", "data": `{"userId":"fresh","userToken":"fresh-token","userTokenExpireTime":"2100000000"}`}}, nil
	}
	return state, o
}
func openService(t *testing.T, o Options) *Service {
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
func cached(o *Options) *conf.User {
	return &conf.User{UserId: "cached", UserToken: "cached-token", UserTokenExpireTime: strconv.FormatInt(o.Now().Add(96*time.Hour).Unix(), 10), UserTitles: []*conf.UserTitle{{Name: "title"}}}
}

func TestCloudLoginRegionAndTwoFactorProtocol(t *testing.T) {
	state, o := fixture(t)
	calls := 0
	o.Request = func(ctx context.Context, region int, route string, h, b map[string]string) (Response, error) {
		calls++
		if region != 1 || state.Region != 1 {
			t.Fatal("region was not committed before request")
		}
		if route == "/apis/siyuan/login" {
			if b["userName"] != "name" || b["userPassword"] != "pw" || b["captcha"] != "cap" {
				t.Fatal(b)
			}
			return Response{200, map[string]any{"code": float64(-1), "msg": "captcha", "needCaptcha": true, "userName": "name", "token": "token"}}, nil
		}
		if route != "/apis/siyuan/login/2fa" || h["token"] != "token" || b["twofactorAuthCode"] != "123456" {
			t.Fatal(route, h, b)
		}
		return Response{200, map[string]any{"code": float64(0), "data": "ok"}}, nil
	}
	s := openService(t, *o)
	r, e := s.Login(context.Background(), "name", "pw", "cap", 1)
	if e != nil || r.Code != 1 || r.Data["needCaptcha"] != true || state.User != nil {
		t.Fatal(r, e)
	}
	r2, e := s.Login2FA(context.Background(), "token", "123456")
	if e != nil || r2["data"] != "ok" || calls != 2 {
		t.Fatal(r2, e, calls)
	}
}
func TestCloudRefreshHydrationExpiryAndIsolation(t *testing.T) {
	state, o := fixture(t)
	u := cached(o)
	state.Cipher, _ = o.Encode(u)
	calls, saves := 0, 0
	commit := o.Commit
	o.Commit = func(ctx context.Context, p Change) error {
		saves++
		if saves == 1 && !p.RuntimeOnly {
			t.Fatal("hydration persisted")
		}
		return commit(ctx, p)
	}
	request := o.Request
	o.Request = func(ctx context.Context, r int, p string, h, b map[string]string) (Response, error) {
		calls++
		return request(ctx, r, p, h, b)
	}
	s := openService(t, *o)
	got, e := s.Refresh(context.Background(), "")
	if e != nil || got.UserId != "cached" || calls != 0 || saves != 1 {
		t.Fatal(got, e, calls, saves)
	}
	got.UserTitles[0].Name = "poison"
	if state.User.UserTitles[0].Name != "title" {
		t.Fatal("borrowed user")
	}
	got, e = s.RefreshCurrent(context.Background())
	if e != nil || got.UserId != "fresh" || state.User.UserId != "fresh" || calls != 1 || saves != 2 {
		t.Fatal(got, e)
	}
	got.UserToken = "poison"
	snapshot, _ := s.Snapshot(context.Background())
	if snapshot.User.UserToken != "fresh-token" {
		t.Fatal("response mutated authority")
	}
	if e = s.Logout(context.Background()); e != nil {
		t.Fatal(e)
	}
	got, e = s.RefreshCurrent(context.Background())
	if e != nil || got != nil || calls != 1 {
		t.Fatal("logout resurrected account", got, e, calls)
	}
}
func TestCloudRefreshTransientInvalidAndMalformedResponses(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	o.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		return Response{}, errors.New("offline")
	}
	s := openService(t, *o)
	u, e := s.Refresh(context.Background(), "explicit")
	if e != nil || u.UserId != "cached" {
		t.Fatal(u, e)
	}
	state.User.UserTokenExpireTime = "bad"
	if _, e = s.Refresh(context.Background(), "explicit"); !errors.Is(e, ErrSession) {
		t.Fatal(e)
	}
	state.User = cached(o)
	o2 := *o
	o2.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
		return Response{200, map[string]any{"code": float64(255)}}, nil
	}
	s2 := openService(t, o2)
	if _, e = s2.Refresh(context.Background(), "explicit"); !errors.Is(e, ErrSession) {
		t.Fatal(e)
	}
	state.User = nil
	state.Cipher = "corrupt"
	if _, e = s.Refresh(context.Background(), ""); !errors.Is(e, ErrSession) {
		t.Fatal(e)
	}
	for _, body := range []map[string]any{nil, {"code": "0"}, {"code": float64(.5)}, {"code": float64(0), "msg": nil}} {
		o3 := *o
		o3.Request = func(context.Context, int, string, map[string]string, map[string]string) (Response, error) {
			return Response{200, body}, nil
		}
		v := openService(t, o3)
		if _, e = v.Login(context.Background(), "", "", "", 0); !errors.Is(e, ErrResponse) {
			t.Fatal(body, e)
		}
	}
}
func TestCloudPersistenceFailureAndCancellation(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	sentinel := errors.New("save failed")
	o.Commit = func(context.Context, Change) error { return sentinel }
	calls := 0
	request := o.Request
	o.Request = func(ctx context.Context, r int, p string, h, b map[string]string) (Response, error) {
		calls++
		return request(ctx, r, p, h, b)
	}
	s := openService(t, *o)
	if _, e := s.Login(context.Background(), "", "", "", 1); !errors.Is(e, sentinel) || calls != 0 {
		t.Fatal(e, calls)
	}
	if _, e := s.Refresh(context.Background(), "explicit"); !errors.Is(e, sentinel) || state.User.UserId != "cached" {
		t.Fatal(e, state.User)
	}
	if e := s.Logout(context.Background()); !errors.Is(e, sentinel) || state.User == nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.Refresh(ctx, "explicit"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := New(Options{}); !errors.Is(e, ErrDependency) {
		t.Fatal(e)
	}
}
func TestCloudLogoutSerializesWithRefreshAndCloseCancels(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	entered, release := make(chan struct{}), make(chan struct{})
	request := o.Request
	o.Request = func(ctx context.Context, r int, p string, h, b map[string]string) (Response, error) {
		close(entered)
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		case <-release:
		}
		return request(ctx, r, p, h, b)
	}
	s := openService(t, *o)
	refresh := make(chan error, 1)
	go func() { _, e := s.Refresh(context.Background(), "explicit"); refresh <- e }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Logout(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	logout := make(chan error, 1)
	go func() { logout <- s.Logout(context.Background()) }()
	close(release)
	if e := <-refresh; e != nil {
		t.Fatal(e)
	}
	if e := <-logout; e != nil {
		t.Fatal(e)
	}
	v, _ := s.Snapshot(context.Background())
	if v.User != nil || v.Cipher != "" {
		t.Fatal("late refresh restored logout", v)
	}
	state2, o2 := fixture(t)
	state2.User = cached(o2)
	started := make(chan struct{})
	o2.Request = func(ctx context.Context, _ int, _ string, _, _ map[string]string) (Response, error) {
		close(started)
		<-ctx.Done()
		return Response{}, ctx.Err()
	}
	s2 := openService(t, *o2)
	done := make(chan error, 1)
	go func() { _, e := s2.RefreshCurrent(context.Background()); done <- e }()
	<-started
	if e := s2.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := s2.Snapshot(context.Background()); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	if state2.User.UserId != "cached" {
		t.Fatal("cancelled request committed")
	}
}
func TestCloudCloseTimeoutIsRetryable(t *testing.T) {
	_, o := fixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	request := o.Request
	o.Request = func(ctx context.Context, r int, p string, h, b map[string]string) (Response, error) {
		close(entered)
		<-release
		return request(ctx, r, p, h, b)
	}
	s := openService(t, *o)
	done := make(chan error, 1)
	go func() { _, e := s.Refresh(context.Background(), "explicit"); done <- e }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e := s.Close(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	close(release)
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
}
