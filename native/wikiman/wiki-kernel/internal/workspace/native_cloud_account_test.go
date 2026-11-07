//go:build fts5

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudaccount"
)

func TestNativeCloudAccountIdentityIsolationAndOwnedDelay(t *testing.T) {
	root := fixture(t)
	before := digest(t, root)
	states := map[string]*cloudaccount.State{pageA: {Region: 0}, pageB: {Region: 1}}
	calls := map[string]int{}
	m := open(t, root, Options{CloudAccountOptions: func(store, page string) (cloudaccount.Options, error) {
		if store != storeID {
			t.Fatal(store)
		}
		state := states[page]
		return cloudaccount.Options{
			Load: func(context.Context) (cloudaccount.State, error) { return *state, nil },
			Commit: func(ctx context.Context, p cloudaccount.Change) error {
				if e := ctx.Err(); e != nil {
					return e
				}
				if p.Region != nil {
					state.Region = *p.Region
				}
				if p.SetUser {
					state.Cipher = p.Cipher
					state.User = p.User
				}
				return nil
			},
			Request: func(ctx context.Context, region int, route string, h, b map[string]string) (cloudaccount.Response, error) {
				calls[page]++
				if route == "/apis/siyuan/login" {
					return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0), "msg": "", "token": "fixture-" + page}}, nil
				}
				raw, _ := json.Marshal(conf.User{UserId: page, UserToken: "fixture-" + page, UserTokenExpireTime: "2100000000"})
				return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0), "data": string(raw)}}, nil
			},
			Decode: func(value string, _ bool) (u *conf.User, e error) { e = json.Unmarshal([]byte(value), &u); return }, Encode: func(u *conf.User) (string, error) { raw, e := json.Marshal(u); return string(raw), e }, Now: time.Now,
		}, nil
	}})
	a, b := getDocLease(t, m, pageA), getDocLease(t, m, pageB)
	ctx := context.Background()
	for l, page := range map[*Lease]string{a: pageA, b: pageB} {
		r, e := l.NativeCloudLogin(ctx, "name", "pw", "", 1)
		if e != nil || r.Data["token"] != "fixture-"+page {
			t.Fatal(r, e)
		}
		u, e := l.NativeCloudRefresh(ctx, "fixture-"+page)
		if e != nil || u.UserId != page {
			t.Fatal(u, e)
		}
		u.UserToken = "poison"
		if states[page].User.UserToken != "fixture-"+page {
			t.Fatal("borrowed user")
		}
	}
	if e := a.NativeCloudLogout(ctx); e != nil {
		t.Fatal(e)
	}
	if states[pageA].User != nil || states[pageB].User == nil {
		t.Fatal("foreign logout")
	}
	task, e := b.StartNativeCloudRefresh(ctx, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	a.Release()
	b.Release()
	if e = m.Drain(ctx); e != nil {
		t.Fatal(e)
	}
	if e = task.Wait(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if calls[pageA] != 2 || calls[pageB] != 2 || digest(t, root) != before {
		t.Fatal("late task or source mutation", calls)
	}
	if _, e = a.NativeCloudRefresh(ctx, "old"); !errors.Is(e, ErrStale) {
		t.Fatal(e)
	}
}
func TestNativeCloudAccountMissingAuthorityAndConstructorFailure(t *testing.T) {
	m := open(t, fixture(t), Options{})
	l := getDocLease(t, m, pageA)
	if e := l.NativeCloudLogout(context.Background()); !errors.Is(e, cloudaccount.ErrDependency) {
		t.Fatal(e)
	}
	bad := open(t, fixture(t), Options{CloudAccountOptions: func(string, string) (cloudaccount.Options, error) { return cloudaccount.Options{}, nil }})
	if _, e := bad.Acquire(context.Background(), storeID, pageA); !errors.Is(e, cloudaccount.ErrDependency) || bad.Stats().Workspaces != 0 {
		t.Fatal(e, bad.Stats())
	}
}
