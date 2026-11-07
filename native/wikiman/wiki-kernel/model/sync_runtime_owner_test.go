// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"wiki-kernel/conf"
	"wiki-kernel/internal/taskgroup"
)

func TestSyncPlanUpdateKeepsCapturedOwner(t *testing.T) {
	old := Conf
	a, b := &AppConf{}, &AppConf{}
	Conf = b
	defer func() { Conf = old }()

	a.planSyncAfter(time.Minute)
	a.syncRuntime.planMu.Lock()
	aPlan := a.syncRuntime.plan
	a.syncRuntime.planMu.Unlock()
	b.syncRuntime.planMu.Lock()
	bPlan := b.syncRuntime.plan
	b.syncRuntime.planMu.Unlock()
	if aPlan.IsZero() || !bPlan.IsZero() {
		t.Fatal("captured owner plan leaked to replacement owner")
	}
}

func TestStorageWaitKeepsCapturedOwnerAfterReplacement(t *testing.T) {
	old := Conf
	a, b := &AppConf{}, &AppConf{}
	a.syncRuntime.booting.Store(true)
	Conf = a
	defer func() { Conf = old }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- waitForSyncingStoragesOwnerContext(ctx, a) }()
	Conf = b
	select {
	case err := <-done:
		t.Fatalf("wait followed replacement owner: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("captured wait did not cancel: %v", err)
	}
}

func TestSyncWebSocketIdentityAndPortsKeepCapturedOwner(t *testing.T) {
	old := Conf
	owner := &AppConf{AppData: conf.AppData{
		Sync: &conf.Sync{CloudName: "captured-repo"},
		User: &conf.User{UserId: "captured-user"},
	}}
	replacement := &AppConf{AppData: conf.AppData{
		Sync: &conf.Sync{CloudName: "replacement-repo"},
		User: &conf.User{UserId: "replacement-user"},
	}}
	Conf = owner
	defer func() { Conf = old }()

	identity, err := captureSyncWebSocketIdentity(owner)
	if err != nil {
		t.Fatal(err)
	}
	ports := ownerSyncWebSocketPorts(owner, identity)
	Conf = replacement
	if ports.user() {
		t.Fatal("old websocket accepted replacement workspace")
	}
	if got := identity.header.Get("x-siyuan-uid"); got != "captured-user" {
		t.Fatalf("identity borrowed replacement user: %q", got)
	}
	if got := identity.header.Get("x-siyuan-repo"); got != "captured-repo" {
		t.Fatalf("identity borrowed replacement repo: %q", got)
	}
}

func TestSyncWebSocketWorkerIsJoinedByRepoTasks(t *testing.T) {
	conn := newOwnedSyncWebSocket()
	owner := &AppConf{repoTasks: taskgroup.Open()}
	done := make(chan struct{})

	webSocketConnLock.Lock()
	webSocketConn, syncWebSocketWorker, syncWebSocketOwner = conn, done, owner
	closedSyncWebSocket.Store(false)
	webSocketConnLock.Unlock()
	if !owner.repoTasks.Go(func(ctx context.Context) {
		runSyncWebSocket(ctx, conn, done, syncWebSocketPorts{})
	}) {
		t.Fatal("websocket worker was not admitted")
	}
	owner.repoTasks.Stop()
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := owner.repoTasks.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
	if currentSyncWebSocket() != nil || conn.closeCount.Load() == 0 {
		t.Fatal("repo task close did not drain websocket")
	}
}

func TestBootSyncWorkersCancelPeersAndJoin(t *testing.T) {
	sentinel := errors.New("index failed")
	fetchStarted := make(chan struct{})
	fetchJoined := make(chan struct{})
	err := runBootSyncWorkers(context.Background(),
		func(context.Context) error {
			<-fetchStarted
			return sentinel
		},
		func(ctx context.Context) error {
			close(fetchStarted)
			<-ctx.Done()
			close(fetchJoined)
			return ctx.Err()
		},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("wrong group error: %v", err)
	}
	select {
	case <-fetchJoined:
	default:
		t.Fatal("failed boot worker returned before its peer joined")
	}
	if err := runBootSyncWorkers(context.Background(), func(context.Context) error { panic("fixture") }); err == nil {
		t.Fatal("boot worker panic was not converted to an error")
	}
}
