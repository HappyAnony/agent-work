// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"wiki-kernel/conf"
	"wiki-kernel/task"
	"wiki-kernel/util"
)

func delayedUIOwnerFixture(t *testing.T) *AppConf {
	t.Helper()
	root := t.TempDir()
	owner := settingOwner(t, filepath.Join(root, "conf.json"), true)
	owner.business = conf.NewBusiness(nil, map[string]map[int]string{
		"en_US": {52: "owned", 237: "updated %d"},
		"zh_CN": {52: "replacement", 237: "replacement %d"},
	}, nil)
	owner.Lang = "en_US"
	previousOwner := Conf
	previousWorkspace, previousData, previousWorking := util.WorkspaceDir, util.DataDir, util.WorkingDir
	Conf, util.WorkspaceDir, util.DataDir, util.WorkingDir = owner, root, filepath.Join(root, "data"), filepath.Join(root, "working")
	t.Cleanup(func() {
		_ = owner.CloseCloudRepo(context.Background())
		owner.repoMu.Lock()
		owner.repoClosed = false
		owner.repoMu.Unlock()
		Conf, util.WorkspaceDir, util.DataDir, util.WorkingDir = previousOwner, previousWorkspace, previousData, previousWorking
	})
	return owner
}

func TestDelayedUIOwnerFreezesLanguageIdentityAndRoots(t *testing.T) {
	owner := delayedUIOwnerFixture(t)
	session, err := captureLegacyDelayedUIOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	owner.Lang = "zh_CN"
	if got := session.language(52, true); got != "owned" {
		t.Fatal("language was not frozen", got)
	}
	var calls atomic.Int32
	if !session.publish(context.Background(), func() { calls.Add(1) }) || calls.Load() != 1 {
		t.Fatal("current owner did not publish")
	}
	replacement := NewAppConf()
	Conf = replacement
	if session.publish(context.Background(), func() { calls.Add(1) }) {
		t.Fatal("replacement owner borrowed delayed publication")
	}
	Conf = owner
	data := util.DataDir
	util.DataDir = data + "-replacement"
	if session.publish(context.Background(), func() { calls.Add(1) }) {
		t.Fatal("replacement data root admitted delayed publication")
	}
	util.DataDir = data
	owner.repoMu.Lock()
	owner.repoClosed = true
	owner.repoMu.Unlock()
	if session.publish(context.Background(), func() { calls.Add(1) }) {
		t.Fatal("closed owner admitted delayed publication")
	}
	if err = session.schedule(context.Background(), task.PushMsg, 0, func() {}, "closed"); err == nil {
		t.Fatal("closed owner admitted delayed task")
	}
	if _, err = captureLegacyDelayedUIOwner(context.Background()); err == nil {
		t.Fatal("closed owner was captured")
	}
}

func TestDelayedUIOwnerDelayCoalescingAndCancellation(t *testing.T) {
	delayedUIOwnerFixture(t)
	session, err := captureLegacyDelayedUIOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	if err = session.schedule(context.Background(), task.ReloadAttributeView, 10*time.Millisecond, func() { calls.Add(1) }, "av"); err != nil {
		t.Fatal(err)
	}
	if err = session.schedule(context.Background(), task.ReloadAttributeView, 10*time.Millisecond, func() { calls.Add(100) }, "av"); err != nil {
		t.Fatal(err)
	}
	if err = session.schedule(context.Background(), task.PushMsg, 10*time.Millisecond, func() { calls.Add(10) }, "notice"); err != nil {
		t.Fatal(err)
	}
	if err = session.schedule(context.Background(), task.PushMsg, 10*time.Millisecond, func() { calls.Add(100) }, "notice"); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	if err = session.schedule(cancelled, task.PushMsg, 10*time.Millisecond, func() { calls.Add(1000) }, "notice"); err != nil {
		t.Fatal(err)
	}
	cancel()
	if calls.Load() != 0 {
		t.Fatal("delayed publication ran early", calls.Load())
	}
	time.Sleep(15 * time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 111 {
		t.Fatal("coalescing or cancellation changed", calls.Load())
	}
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if err = session.schedule(ctx, task.PushMsg, 0, func() {}, "late"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled admission accepted", err)
	}
}

func TestDelayedUIOwnerCloseCancelsAndClearsPending(t *testing.T) {
	owner := delayedUIOwnerFixture(t)
	session, err := captureLegacyDelayedUIOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	if err = session.schedule(context.Background(), task.ReloadFiletree, time.Hour, func() { calls.Add(1) }, "tree"); err != nil {
		t.Fatal(err)
	}
	owner.repoMu.Lock()
	pending := len(owner.repoDelayed)
	owner.repoMu.Unlock()
	if pending != 1 {
		t.Fatal("delayed task was not tracked", pending)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = owner.CloseCloudRepo(ctx); err != nil {
		t.Fatal(err)
	}
	owner.repoMu.Lock()
	pending = len(owner.repoDelayed)
	owner.repoMu.Unlock()
	if pending != 0 || calls.Load() != 0 {
		t.Fatal("close did not cancel and clear delayed task", pending, calls.Load())
	}
	if err = session.schedule(context.Background(), task.ReloadFiletree, 0, func() {}, "after-close"); err == nil {
		t.Fatal("closed owner admitted delayed task")
	}
}

func TestRequestFollowupSchedulesUseAndDrainCapturedOwner(t *testing.T) {
	owner := delayedUIOwnerFixture(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if ScheduleLegacyUIReloadContext(cancelled) || ScheduleUserGuideOpenContext(cancelled, "missing", "app") {
		t.Fatal("cancelled request admitted follow-up work")
	}
	if ScheduleUserGuideOpenContext(context.Background(), "", "app") {
		t.Fatal("empty guide start ID admitted polling")
	}
	if !ScheduleLegacyUIReloadContext(context.Background()) {
		t.Fatal("UI reload follow-up was rejected")
	}
	if !ScheduleUserGuideOpenContext(context.Background(), "missing", "app") {
		t.Fatal("user guide follow-up was rejected")
	}
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := owner.CloseCloudRepo(ctx); err != nil {
		t.Fatal(err)
	}
	if ScheduleLegacyUIReloadContext(context.Background()) || ScheduleUserGuideOpenContext(context.Background(), "missing", "app") {
		t.Fatal("closed owner admitted request follow-up work")
	}
}
