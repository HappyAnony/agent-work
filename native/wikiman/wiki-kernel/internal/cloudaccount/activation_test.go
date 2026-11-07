package cloudaccount

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestActivationProtocolNormalizationAndNoLocalMutation(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	state.Region = 1
	calls := 0
	wantRoute := "/apis/siyuan/checkActivationcode"
	next := Response{Status: 200, Body: map[string]any{"code": float64(0), "msg": "confirm fixture"}}
	o.Commit = func(context.Context, Change) error { t.Fatal("activation committed local account"); return nil }
	o.Request = func(ctx context.Context, region int, route string, h, body map[string]string) (Response, error) {
		calls++
		if region != 1 || route != wantRoute || h["Cookie"] != "symphony=cached-token" || len(body) != 1 || body["data"] != "AB-CD" {
			t.Fatal("original activation wire changed")
		}
		return next, nil
	}
	s := openService(t, *o)
	input := " \tAB\u200b-\ue000CD\n "
	v, e := s.CheckActivationcode(context.Background(), input)
	if e != nil || v.Code != 0 || v.Msg != "confirm fixture" || v.Data != nil {
		t.Fatal(v, e)
	}
	for _, n := range []float64{-1, 1, 42} {
		next.Body["code"] = n
		v, e = s.CheckActivationcode(context.Background(), input)
		if e != nil || v.Code != 1 || v.Msg != "confirm fixture" {
			t.Fatal(v, e)
		}
	}
	wantRoute = "/apis/siyuan/useActivationcode"
	var business *BusinessError
	if e = s.UseActivationcode(context.Background(), input); !errors.As(e, &business) || business.Message != "confirm fixture" {
		t.Fatal(e)
	}
	next.Body["code"] = float64(0)
	if e = s.UseActivationcode(context.Background(), input); e != nil {
		t.Fatal(e)
	}
	if calls != 6 || state.User.UserId != "cached" || state.User.UserToken != "cached-token" || state.Cipher != "" || state.Region != 1 {
		t.Fatal("local account changed")
	}
}

func TestActivationMalformedFailuresAndTokenInvalidation(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	next := Response{Status: 200, Body: map[string]any{"code": float64(0), "msg": "ok"}}
	tokens := 0
	requests := 0
	var transportErr error
	o.Request = func(ctx context.Context, _ int, route string, _, _ map[string]string) (Response, error) {
		requests++
		if route == "/apis/siyuan/upload/token" {
			tokens++
			return tokenReply("synthetic"), nil
		}
		return next, transportErr
	}
	s := openService(t, *o)
	get := func(want int) {
		t.Helper()
		if _, e := s.UploadToken(context.Background()); e != nil || tokens != want {
			t.Fatal(e, tokens, want)
		}
	}
	get(1)
	if _, e := s.CheckActivationcode(context.Background(), "fixture"); e != nil {
		t.Fatal(e)
	}
	get(1)
	if e := s.UseActivationcode(context.Background(), "fixture"); e != nil {
		t.Fatal(e)
	}
	get(2)
	for _, r := range []Response{{Status: 200}, {Status: 200, Body: map[string]any{"code": "0", "msg": "bad"}}, {Status: 200, Body: map[string]any{"code": float64(0), "msg": []string{"bad"}}}, {Status: 200, Body: map[string]any{"code": 0.5, "msg": "bad"}}, {Status: 503}, {Status: 401}} {
		next = r
		if _, e := s.CheckActivationcode(context.Background(), "fixture"); e == nil {
			t.Fatal("accepted malformed check")
		}
		if e := s.UseActivationcode(context.Background(), "fixture"); e == nil || s.upload.valid {
			t.Fatal("accepted malformed use or retained credential", e)
		}
	}
	next = Response{Status: 401}
	if _, e := s.CheckActivationcode(context.Background(), "fixture"); !errors.Is(e, ErrNetwork) {
		t.Fatal("original 401 display changed", e)
	}
	transportErr = errors.New("sensitive remote transport detail")
	if e := s.UseActivationcode(context.Background(), "fixture"); !errors.Is(e, ErrNetwork) {
		t.Fatal("transport error not sanitized", e)
	}
	state.User = nil
	before := requests
	if _, e := s.CheckActivationcode(context.Background(), "fixture"); !errors.Is(e, ErrAuthentication) {
		t.Fatal(e)
	}
	if e := s.UseActivationcode(context.Background(), "fixture"); !errors.Is(e, ErrAuthentication) || requests != before {
		t.Fatal(e, requests)
	}
}

func TestActivationCancellationSerializesLogoutAndJoinsOwner(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	entered, exited := make(chan struct{}), make(chan struct{})
	o.Request = func(ctx context.Context, _ int, route string, _, _ map[string]string) (Response, error) {
		if route == "/apis/siyuan/upload/token" {
			return tokenReply("synthetic"), nil
		}
		close(entered)
		<-ctx.Done()
		close(exited)
		return Response{}, ctx.Err()
	}
	s := openService(t, *o)
	if _, e := s.UploadToken(context.Background()); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.UseActivationcode(ctx, "fixture") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("activation never started")
	}
	deadline, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if e := s.Logout(deadline); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("logout interleaved with redemption", e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("close did not join IO")
	}
	if e := <-done; !errors.Is(e, context.Canceled) || s.upload.valid || state.User == nil {
		t.Fatal("uncertain redemption retained credential or changed user", e)
	}
	if _, e := s.CheckActivationcode(context.Background(), "fixture"); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
}
