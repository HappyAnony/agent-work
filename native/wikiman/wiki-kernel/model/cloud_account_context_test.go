// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/cloudaccount"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/util"
)

func cloudFixture(t *testing.T) (*AppConf, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "conf.json")
	c, e := NewAppConfAt(file, false)
	if e != nil {
		t.Fatal(e)
	}
	c.Lang = "en_US"
	return c, file
}
func remoteUser(t *testing.T, u *conf.User) string {
	t.Helper()
	raw, e := json.Marshal(u)
	if e != nil {
		t.Fatal(e)
	}
	block, _ := aes.NewCipher(util.SK)
	padding := block.BlockSize() - len(raw)%block.BlockSize()
	for i := 0; i < padding; i++ {
		raw = append(raw, byte(padding))
	}
	out := make([]byte, len(raw))
	cipher.NewCBCEncrypter(block, []byte("RandomInitVector")).CryptBlocks(out, raw)
	return hex.EncodeToString(out)
}
func TestCloudAccountOwnerPersistenceCodecAndCopies(t *testing.T) {
	c, file := cloudFixture(t)
	previous := Conf
	Conf = &AppConf{AppData: conf.AppData{CloudRegion: 9}}
	defer func() { Conf = previous }()
	u := &conf.User{UserId: "synthetic", UserToken: "synthetic-token", UserTokenExpireTime: "2100000000", UserTitles: []*conf.UserTitle{{Name: "original"}}}
	encrypted := remoteUser(t, u)
	calls := 0
	s, e := c.newCloudAccount(func(ctx context.Context, r int, p string, h, b map[string]string) (cloudaccount.Response, error) {
		calls++
		if r != 1 {
			t.Fatal(r)
		}
		if p == "/apis/siyuan/login" {
			return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0), "msg": "", "token": "synthetic-token"}}, nil
		}
		return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0), "data": encrypted}}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close(context.Background())
	if _, e = s.Login(context.Background(), "name", "pw", "", 1); e != nil {
		t.Fatal(e)
	}
	if c.GetUser() != nil || Conf.CloudRegion != 9 {
		t.Fatal("login published user or borrowed Conf")
	}
	got, e := s.Refresh(context.Background(), "synthetic-token")
	if e != nil || got.UserId != u.UserId {
		t.Fatal(got, e)
	}
	got.UserTitles[0].Name = "poison"
	copy := c.GetUser()
	copy.UserToken = "poison"
	if c.GetUser().UserToken != u.UserToken || c.GetUser().UserTitles[0].Name != "original" {
		t.Fatal("borrowed account")
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	var disk conf.AppData
	if e = json.Unmarshal(raw, &disk); e != nil {
		t.Fatal(e)
	}
	decoded, e := decodeCloudUser(disk.UserData, true)
	if e != nil || decoded.UserToken != u.UserToken || disk.User != nil {
		t.Fatal(decoded, e)
	}
	if e = s.Logout(context.Background()); e != nil {
		t.Fatal(e)
	}
	if c.UserData != "" || c.GetUser() != nil || calls != 2 {
		t.Fatal("logout did not clear owner")
	}
	for _, bad := range []string{"", "00", "not-hex", encrypted[:len(encrypted)-2]} {
		if _, e := decodeCloudUser(bad, false); e == nil {
			t.Fatal("accepted broken ciphertext")
		}
	}
}
func TestCloudAccountSaveFailureHydrationAndReadOnly(t *testing.T) {
	c, file := cloudFixture(t)
	u := &conf.User{UserId: "cached", UserTokenExpireTime: strconv.FormatInt(time.Now().Add(96*time.Hour).Unix(), 10)}
	raw, _ := json.Marshal(u)
	c.UserData = util.AESEncrypt(string(raw))
	if e := c.SaveError(); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(file)
	calls := 0
	s, e := c.newCloudAccount(func(context.Context, int, string, map[string]string, map[string]string) (cloudaccount.Response, error) {
		calls++
		return cloudaccount.Response{}, errors.New("offline")
	})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close(context.Background())
	if _, e = s.Refresh(context.Background(), ""); e != nil || c.GetUser() == nil || calls != 0 {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Fatal("hydration rewrote config")
	}
	c.confPath = filepath.Join(file, "impossible.json")
	if e = s.Logout(context.Background()); e == nil || c.GetUser() == nil || c.UserData == "" {
		t.Fatal("failed save changed memory", e)
	}
	c.confPath = file
	c.persistenceReadOnly = true
	if _, e = s.Login(context.Background(), "", "", "", 1); !errors.Is(e, ErrSettingReadOnly) || calls != 0 {
		t.Fatal(e, calls)
	}
	readOnlyService, e := c.newCloudAccount(func(context.Context, int, string, map[string]string, map[string]string) (cloudaccount.Response, error) {
		return cloudaccount.Response{Status: 200, Body: map[string]any{"code": float64(0), "data": remoteUser(t, &conf.User{UserId: "readonly-refresh"})}}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	defer readOnlyService.Close(context.Background())
	if _, e = readOnlyService.Refresh(context.Background(), "fixture"); e != nil || c.GetUser().UserId != "readonly-refresh" {
		t.Fatal(e)
	}
	after, _ = os.ReadFile(file)
	if string(before) != string(after) {
		t.Fatal("read-only refresh wrote credentials")
	}
	input := &conf.User{UserId: "input", UserTitles: []*conf.UserTitle{{Name: "title"}}}
	c.SetUser(input)
	input.UserTitles[0].Name = "poison"
	if c.GetUser().UserTitles[0].Name != "title" {
		t.Fatal("SetUser borrowed caller")
	}
}
func TestCloudAccountDelayedTaskCloseAndLogout(t *testing.T) {
	c, _ := cloudFixture(t)
	calls := make(chan struct{}, 1)
	s, e := c.newCloudAccount(func(ctx context.Context, _ int, _ string, _, _ map[string]string) (cloudaccount.Response, error) {
		calls <- struct{}{}
		<-ctx.Done()
		return cloudaccount.Response{}, ctx.Err()
	})
	if e != nil {
		t.Fatal(e)
	}
	c.cloudAccount = s
	c.cloudTasks = taskgroup.Open()
	c.SetUser(&conf.User{UserToken: "synthetic"})
	if e = c.ScheduleCloudRefresh(time.Hour); e != nil {
		t.Fatal(e)
	}
	if e = c.CloseCloudAccount(context.Background()); e != nil {
		t.Fatal(e)
	}
	select {
	case <-calls:
		t.Fatal("cancelled delay sent request")
	default:
	}
	if e = c.ScheduleCloudRefresh(0); !errors.Is(e, cloudaccount.ErrClosed) {
		t.Fatal(e)
	}
	c2, _ := cloudFixture(t)
	s2, e := c2.newCloudAccount(func(context.Context, int, string, map[string]string, map[string]string) (cloudaccount.Response, error) {
		t.Error("logged-out delayed task used old token")
		return cloudaccount.Response{}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	c2.cloudAccount = s2
	c2.cloudTasks = taskgroup.Open()
	c2.SetUser(&conf.User{UserToken: "old"})
	if e = s2.Logout(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = s2.RefreshCurrent(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = c2.CloseCloudAccount(context.Background()); e != nil {
		t.Fatal(e)
	}
}

func TestCloudAccountActualHTTPWireAndCancellation(t *testing.T) {
	c, _ := cloudFixture(t)
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/siyuan/login":
			if body["userName"] != "fixture" || body["userPassword"] != "pw" || body["captcha"] != "cap" {
				t.Error(body)
			}
			_, _ = w.Write([]byte(`{"code":0,"msg":"","userName":"fixture","token":"fixture-token","needCaptcha":false}`))
		case "/apis/siyuan/login/2fa":
			if r.Header.Get("token") != "fixture-token" || body["twofactorAuthCode"] != "123456" {
				t.Error("two-factor wire mismatch")
			}
			_, _ = w.Write([]byte(`{"code":0,"msg":"","token":"fixture-token"}`))
		case "/apis/siyuan/user":
			close(entered)
			<-r.Context().Done()
		default:
			t.Error("unexpected endpoint", r.URL.Path)
		}
	}))
	defer server.Close()
	s, e := c.newCloudAccount(func(ctx context.Context, region int, route string, h, b map[string]string) (cloudaccount.Response, error) {
		if region != 1 {
			t.Error(region)
		}
		return requestCloudAccountURL(ctx, server.URL+route, h, b)
	})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close(context.Background())
	if v, e := s.Login(context.Background(), "fixture", "pw", "cap", 1); e != nil || v.Data["token"] != "fixture-token" {
		t.Fatal(v, e)
	}
	if v, e := s.Login2FA(context.Background(), "fixture-token", "123456"); e != nil || v["token"] != "fixture-token" {
		t.Fatal(v, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := s.Refresh(ctx, "fixture-token"); done <- e }()
	<-entered
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP request ignored cancellation")
	}
	if c.GetUser() != nil {
		t.Fatal("cancelled refresh committed")
	}
}
