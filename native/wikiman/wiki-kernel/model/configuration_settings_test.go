package model

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
	"wiki-kernel/conf"
)

func settingOwner(t *testing.T, path string, readonly bool) *AppConf {
	t.Helper()
	c, e := NewAppConfAt(path, readonly)
	if e != nil {
		t.Fatal(e)
	}
	c.Editor = conf.NewEditor()
	c.FileTree = conf.NewFileTree()
	c.Search = conf.NewSearch()
	return c
}

func TestSettingOwnerPersistsBeforeEffectsAndNeverBorrows(t *testing.T) {
	root := t.TempDir()
	a := settingOwner(t, filepath.Join(root, "a.json"), false)
	b := settingOwner(t, filepath.Join(root, "b.json"), false)
	old := Conf
	Conf = b
	defer func() { Conf = old }()
	a.Editor.VirtualBlockRef = false
	var order []string
	ports := SettingPorts{}
	for _, kind := range []string{"history", "virtual", "readonly", "markdown"} {
		kind := kind
		ports[kind] = func(ctx context.Context, p conf.SettingPlan, e conf.SettingEffect) error {
			raw, err := os.ReadFile(a.confPath)
			if err != nil {
				return err
			}
			var saved conf.AppData
			if err = json.Unmarshal(raw, &saved); err != nil {
				return err
			}
			if saved.Editor.FontSize != 33 || a.Editor.FontSize != 33 {
				t.Fatal("effect preceded commit")
			}
			order = append(order, kind)
			p.Data.Editor.FontSize = 99
			return nil
		}
	}
	value, e := a.ApplySetting(context.Background(), "editor", []byte(`{"fontSize":33,"generateHistoryInterval":21,"virtualBlockRef":true,"readOnly":true}`), ports)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(order, []string{"history", "virtual", "readonly", "markdown"}) || a.Editor.FontSize != 33 || b.Editor.FontSize == 33 || len(value) == 0 {
		t.Fatal(order, e)
	}
	if _, e = os.Stat(b.confPath); !os.IsNotExist(e) {
		t.Fatal("global owner was saved", e)
	}
	var response conf.Editor
	if e = json.Unmarshal(value, &response); e != nil || response.FontSize != 33 {
		t.Fatal(response, e)
	}
}

func TestSettingOwnerFailureCancellationAndAuthority(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	a := settingOwner(t, filepath.Join(root, "a.json"), false)
	bad := settingOwner(t, filepath.Join(root, "missing", "b.json"), false)
	ro := settingOwner(t, filepath.Join(root, "ro.json"), true)
	called := 0
	ports := SettingPorts{"readonly": func(context.Context, conf.SettingPlan, conf.SettingEffect) error { called++; return nil }}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, tt := range []struct {
		c     *AppConf
		ctx   context.Context
		ports SettingPorts
	}{{a, canceled, ports}, {a, ctx, nil}, {bad, ctx, ports}, {ro, ctx, ports}} {
		if _, e := tt.c.ApplySetting(tt.ctx, "editorReadOnly", []byte(`{"readonly":true}`), tt.ports); e == nil {
			t.Fatal("expected failure")
		}
		if tt.c.Editor.ReadOnly {
			t.Fatal("failure published memory")
		}
	}
	if called != 0 {
		t.Fatal("failed setting published effect")
	}
	if _, e := os.Stat(a.confPath); !os.IsNotExist(e) {
		t.Fatal("unbound effect wrote disk", e)
	}
	detached := NewAppConf()
	detached.Editor = conf.NewEditor()
	if _, e := detached.ApplySetting(ctx, "editorReadOnly", []byte(`{"readonly":true}`), ports); !errors.Is(e, ErrUnboundAppConf) {
		t.Fatal(e)
	}
	failure := errors.New("effect failed")
	ports["readonly"] = func(context.Context, conf.SettingPlan, conf.SettingEffect) error { return failure }
	if _, e := a.ApplySetting(ctx, "editorReadOnly", []byte(`{"readonly":true}`), ports); !errors.Is(e, ErrSettingEffects) || !errors.Is(e, failure) || !a.Editor.ReadOnly {
		t.Fatal("committed failure hidden", e)
	}
	committed, cancelCommitted := context.WithCancel(ctx)
	defer cancelCommitted()
	reindexed := false
	finish := SettingPorts{
		"search": func(context.Context, conf.SettingPlan, conf.SettingEffect) error { cancelCommitted(); return nil },
		"reindex": func(c context.Context, _ conf.SettingPlan, _ conf.SettingEffect) error {
			if c.Err() != nil {
				t.Fatal("request cancellation escaped into committed effects")
			}
			reindexed = true
			return nil
		},
		"virtual": func(context.Context, conf.SettingPlan, conf.SettingEffect) error { return nil },
	}
	if _, e := a.ApplySetting(committed, "search", []byte(`{"caseSensitive":true}`), finish); e != nil || !reindexed {
		t.Fatal("committed config omitted required runtime effects", e)
	}
}

func TestSettingOwnerSerializesConcurrentUpdates(t *testing.T) {
	c := settingOwner(t, filepath.Join(t.TempDir(), "conf.json"), false)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := c.ApplySetting(context.Background(), "flashcard", []byte(`{"newCardLimit":12}`), nil); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	raw, e := os.ReadFile(c.confPath)
	if e != nil {
		t.Fatal(e)
	}
	var saved conf.AppData
	if e = json.Unmarshal(raw, &saved); e != nil || saved.Flashcard.NewCardLimit != 12 || c.Flashcard.NewCardLimit != 12 {
		t.Fatal(e)
	}
}

func TestSettingOwnerCanceledAdmissionDoesNotWaitForEffects(t *testing.T) {
	c := settingOwner(t, filepath.Join(t.TempDir(), "conf.json"), false)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	ports := SettingPorts{"readonly": func(context.Context, conf.SettingPlan, conf.SettingEffect) error {
		close(entered)
		<-release
		return nil
	}}
	go func() {
		_, e := c.ApplySetting(context.Background(), "editorReadOnly", []byte(`{"readonly":true}`), ports)
		done <- e
	}()
	defer func() {
		close(release)
		if e := <-done; e != nil {
			t.Error(e)
		}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first operation did not enter effects")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, e := c.ApplySetting(ctx, "account", []byte(`{}`), nil); finished <- e }()
	select {
	case e := <-finished:
		if !errors.Is(e, context.DeadlineExceeded) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled caller waited for unrelated effect")
	}
	if c.Account != nil {
		t.Fatal("cancelled admission published setting")
	}
}
