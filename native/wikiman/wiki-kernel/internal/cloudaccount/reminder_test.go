package cloudaccount

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"wiki-kernel/internal/reminder"
)

func TestReminderCloudProtocolFailuresAndSubscription(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	state.Region = 1
	state.User.UserSiYuanProExpireTime = -1
	next := successfulBusiness()
	calls := 0
	o.ReminderRequest = func(ctx context.Context, region int, route string, h map[string]string, b map[string]any) (Response, error) {
		calls++
		if region != 1 || route != "/apis/siyuan/calendar/setBlockReminder" || h["Cookie"] != "symphony=cached-token" || len(b) != 3 || b["dataId"] != "fixture" || b["data"] != "content" || b["timed"] != int64(1760000000000) {
			t.Fatal("calendar wire changed")
		}
		return next, nil
	}
	s := openService(t, *o)
	p := reminder.Payload{ID: "fixture", Content: "content", Milliseconds: 1760000000000}
	if e := s.ExecuteBlockReminder(context.Background(), func(c context.Context, send func(reminder.Payload) error) error { return send(p) }); e != nil {
		t.Fatal(e)
	}
	for _, r := range []Response{{Status: 503}, {Status: 401}, {Status: 200}, {Status: 200, Body: map[string]any{"code": "0"}}, {Status: 200, Body: map[string]any{"code": float64(8), "msg": "fixture denied"}}} {
		next = r
		if e := s.SetCloudBlockReminder(context.Background(), p); e == nil {
			t.Fatal("bad response accepted")
		}
	}
	next = Response{Status: 401}
	s.upload.valid = true
	if e := s.SetCloudBlockReminder(context.Background(), p); !errors.Is(e, ErrAuthentication) || s.upload.valid {
		t.Fatal(e)
	}
	state.User.UserSiYuanSubscriptionStatus = 1
	before := calls
	if e := s.ExecuteBlockReminder(context.Background(), func(context.Context, func(reminder.Payload) error) error {
		t.Fatal("unsubscribed workflow ran")
		return nil
	}); !errors.Is(e, ErrSubscription) || calls != before {
		t.Fatal(e)
	}
	state.User = nil
	if e := s.SetCloudBlockReminder(context.Background(), p); !errors.Is(e, ErrAuthentication) || calls != before {
		t.Fatal(e)
	}
}
func TestReminderCloudCloseCancelsAndJoinsActualRequest(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	entered, exited := make(chan struct{}), make(chan struct{})
	o.ReminderRequest = func(ctx context.Context, _ int, _ string, _ map[string]string, _ map[string]any) (Response, error) {
		close(entered)
		<-ctx.Done()
		close(exited)
		return Response{}, ctx.Err()
	}
	s := openService(t, *o)
	done := make(chan error, 1)
	go func() { done <- s.SetCloudBlockReminder(context.Background(), reminder.Payload{ID: "fixture"}) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not enter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e := s.Logout(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if e := s.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	select {
	case <-exited:
	default:
		t.Fatal("did not join")
	}
	if e := <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestSubscriptionReminderOriginalThresholdsAndOneTime(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		offset                    time.Duration
		trial, ios, expired, want bool
		days                      int
	}{{"paid14", 14 * 24 * time.Hour, false, false, false, true, 14}, {"paid15", 15 * 24 * time.Hour, false, false, false, false, 0}, {"trial2", 2 * 24 * time.Hour, true, false, false, true, 2}, {"trial3", 3 * 24 * time.Hour, true, false, false, false, 0}, {"lastDay", 12 * time.Hour, false, false, false, false, 0}, {"expired2", -48 * time.Hour, false, false, true, true, 0}, {"expiredOld", -48*time.Hour - time.Millisecond, false, false, true, false, 0}, {"ios", 24 * time.Hour, false, true, false, false, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			state, o := fixture(t)
			state.User = cached(o)
			state.User.UserSiYuanSubscriptionStatus = 0
			state.User.UserSiYuanProExpireTime = float64(o.Now().Add(tc.offset).UnixMilli())
			if tc.trial {
				state.User.UserSiYuanSubscriptionPlan = 2
			}
			s := openService(t, *o)
			n, e := s.PlanExpirationReminder(context.Background(), tc.ios)
			if e != nil || (n != nil) != tc.want {
				t.Fatal(n, e)
			}
			if n != nil && (n.Days != tc.days || n.Expired != tc.expired || n.Delay != map[bool]time.Duration{true: 30 * time.Second, false: 7 * time.Second}[tc.expired]) {
				t.Fatal(n)
			}
			if again, e := s.PlanExpirationReminder(context.Background(), tc.ios); e != nil || again != nil {
				t.Fatal("duplicate reminder", again, e)
			}
		})
	}
}
func TestSubscriptionReminderRevalidatesIdentityResetAndConcurrentPlanning(t *testing.T) {
	state, o := fixture(t)
	state.User = cached(o)
	state.User.UserSiYuanProExpireTime = float64(o.Now().Add(48 * time.Hour).UnixMilli())
	s := openService(t, *o)
	var mu sync.Mutex
	planned := []ExpirationNotice{}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, e := s.PlanExpirationReminder(context.Background(), false)
			if e != nil {
				t.Error(e)
			}
			if n != nil {
				mu.Lock()
				planned = append(planned, *n)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(planned) != 1 {
		t.Fatal(planned)
	}
	state.User.UserToken = "new-token"
	delivered := 0
	push := func(context.Context, ExpirationNotice) error { delivered++; return nil }
	if e := s.DeliverExpirationReminder(context.Background(), planned[0], push); e != nil || delivered != 0 {
		t.Fatal("old account delivery", e)
	}
	n, e := s.PlanExpirationReminder(context.Background(), false)
	if e != nil || n == nil {
		t.Fatal(n, e)
	}
	if e = s.DeliverExpirationReminder(context.Background(), *n, push); e != nil {
		t.Fatal(e)
	}
	if e = s.DeliverExpirationReminder(context.Background(), *n, push); e != nil || delivered != 1 {
		t.Fatal("duplicate delivery", e)
	}
	if e = s.ResetExpirationReminder(context.Background()); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	e = s.RunExpirationReminder(ctx, false, func(c context.Context, d time.Duration) error { cancel(); return c.Err() }, push)
	if !errors.Is(e, context.Canceled) || delivered != 1 {
		t.Fatal(e, delivered)
	}
}
