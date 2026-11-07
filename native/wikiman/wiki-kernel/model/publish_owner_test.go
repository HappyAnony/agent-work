package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func TestPublishCredentialsStableReadsOwnerRevocationAndCopies(t *testing.T) {
	sessionLock.Lock()
	oldAccounts, oldSessions, oldKey, oldConfig, oldOwner, oldEnabled := accountsMap, sessionsMap, jwtKey, lastPublishAuth, lastPublishOwner, lastPublishEnabled
	sessionLock.Unlock()
	defer func() {
		sessionLock.Lock()
		accountsMap, sessionsMap, jwtKey, lastPublishAuth, lastPublishOwner, lastPublishEnabled = oldAccounts, oldSessions, oldKey, oldConfig, oldOwner, oldEnabled
		sessionLock.Unlock()
	}()
	a, b := NewAppConf(), NewAppConf()
	p := conf.NewPublish()
	p.Enable = true
	p.Auth.Accounts = []*conf.BasicAuthAccount{{Username: "reader", Password: "pw"}}
	if e := ConfigurePublishOwnerAccounts(a, p); e != nil {
		t.Fatal(e)
	}
	first := GetBasicAuthAccount("reader")
	AddSession("session", "reader")
	if e := ConfigurePublishOwnerAccounts(a, p); e != nil {
		t.Fatal(e)
	}
	if GetBasicAuthAccount("reader").Token != first.Token || GetBasicAuthUsernameBySessionID("session") != "reader" {
		t.Fatal("identical getter rotated keys")
	}
	first.Password = "borrowed"
	if GetBasicAuthAccount("reader").Password != "pw" {
		t.Fatal("borrowed account")
	}
	token := GetBasicAuthAccount("reader").Token
	if v, e := ParseJWT(token); e != nil || !v.Valid || GetClaimRole(GetTokenClaims(v)) != RoleReader {
		t.Fatal(e)
	}
	if e := ConfigurePublishOwnerAccounts(b, p); e != nil {
		t.Fatal(e)
	}
	if _, e := ParseJWT(token); e == nil || GetBasicAuthUsernameBySessionID("session") != "" {
		t.Fatal("foreign owner retained old identity")
	}
	token = GetBasicAuthAccount("reader").Token
	p.Enable = false
	if e := ConfigurePublishOwnerAccounts(b, p); e != nil {
		t.Fatal(e)
	}
	if _, e := ParseJWT(token); e == nil {
		t.Fatal("disable did not revoke tokens")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if e := ConfigurePublishOwnerAccounts(b, p); e != nil {
					t.Error(e)
				}
				a := GetBasicAuthAccount("reader")
				if _, e := ParseJWT(a.Token); e != nil {
					t.Error(e)
				}
				AddSession("same", "reader")
				GetBasicAuthUsernameBySessionID("same")
				DeleteSession("same")
			}
		}()
	}
	wg.Wait()
}

func TestPublishSettingPersistsBeforeRuntimeAndGetterSharesGate(t *testing.T) {
	c := settingOwner(t, filepath.Join(t.TempDir(), "conf.json"), false)
	c.Publish = conf.NewPublish()
	raw := []byte(`{"enable":false,"port":0,"auth":{"enable":true,"accounts":[]}}`)
	if _, e := c.ApplySetting(context.Background(), "publish", raw, nil); !errors.Is(e, conf.ErrBusinessUnbound) {
		t.Fatal(e)
	}
	called := false
	ports := SettingPorts{"publish": func(ctx context.Context, p conf.SettingPlan, _ conf.SettingEffect) error {
		called = true
		data, e := os.ReadFile(c.confPath)
		if e != nil {
			return e
		}
		var saved conf.AppData
		if e = json.Unmarshal(data, &saved); e != nil {
			return e
		}
		if saved.Publish.Port != 0 || p.Data.Publish.Auth == nil {
			t.Fatal("runtime preceded save")
		}
		return nil
	}}
	if _, e := c.ApplySetting(context.Background(), "publish", raw, ports); e != nil || !called {
		t.Fatal(e)
	}
	if e := c.WithSettingSnapshot(context.Background(), func(data conf.AppData) error { data.Publish.Port = 42; return nil }); e != nil || c.Publish.Port == 42 {
		t.Fatal("snapshot borrowed config", e)
	}
	if _, e := c.AppData.PrepareSetting("publish", []byte(`{"auth":null}`)); e == nil {
		t.Fatal("nil auth accepted")
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	ports["publish"] = func(context.Context, conf.SettingPlan, conf.SettingEffect) error {
		close(entered)
		<-release
		return nil
	}
	go func() { _, e := c.ApplySetting(context.Background(), "publish", raw, ports); done <- e }()
	defer func() {
		close(release)
		if e := <-done; e != nil {
			t.Error(e)
		}
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("setter not entered")
	}
	deadline, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if e := c.WithSettingSnapshot(deadline, func(conf.AppData) error { t.Fatal("getter bypassed runtime publication"); return nil }); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
}
