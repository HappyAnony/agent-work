// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"wiki-kernel/util"
)

type blockingWorkspaceWatcher struct {
	called  chan struct{}
	release chan struct{}
	done    chan struct{}
	once    sync.Once
}

func newBlockingWorkspaceWatcher() *blockingWorkspaceWatcher {
	return &blockingWorkspaceWatcher{called: make(chan struct{}), release: make(chan struct{}), done: make(chan struct{})}
}

func (w *blockingWorkspaceWatcher) Close(ctx context.Context) error {
	w.once.Do(func() { close(w.called) })
	select {
	case <-w.release:
		select {
		case <-w.done:
		default:
			close(w.done)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *blockingWorkspaceWatcher) Done() <-chan struct{} { return w.done }

func TestRepoBackgroundRejectsReplacementAndJoinsAcceptedWork(t *testing.T) {
	old := Conf
	owner, replacement := &AppConf{}, &AppConf{}
	Conf = owner
	defer func() { Conf = old }()

	started := make(chan struct{})
	release := make(chan struct{})
	if !owner.startRepoBackground(func(context.Context) {
		close(started)
		<-release
	}) {
		t.Fatal("background work was not admitted")
	}
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- owner.CloseCloudRepo(context.Background()) }()
	select {
	case err := <-closeDone:
		t.Fatalf("owner close returned before callback: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	Conf = replacement
	if owner.startRepoBackground(func(context.Context) {}) || owner.repoTasks == nil {
		t.Fatal("closed owner admitted replacement work")
	}
}

func TestWorkspaceWatcherOwnerCloseIsRetryableAndGuardsRoots(t *testing.T) {
	oldConf := Conf
	oldWorkspace, oldData, oldTemp, oldWorking := util.WorkspaceDir, util.DataDir, util.TempDir, util.WorkingDir
	owner := &AppConf{}
	Conf = owner
	util.WorkspaceDir, util.DataDir, util.TempDir, util.WorkingDir = "/fixture/workspace", "/fixture/data", "/fixture/temp", "/fixture/working"
	defer func() {
		Conf = oldConf
		util.WorkspaceDir, util.DataDir, util.TempDir, util.WorkingDir = oldWorkspace, oldData, oldTemp, oldWorking
	}()

	captured, err := captureWorkspaceWatcherOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	watcher := newBlockingWorkspaceWatcher()
	if !captured.ownWorkspaceWatcher(watcher) {
		t.Fatal("watcher was not admitted")
	}
	deadline, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if err = owner.CloseCloudRepo(deadline); !errors.Is(err, context.DeadlineExceeded) {
		cancel()
		t.Fatalf("expected retryable close timeout, got %v", err)
	}
	cancel()
	select {
	case <-watcher.called:
	default:
		t.Fatal("owner cancellation did not close watcher")
	}
	close(watcher.release)
	if err = owner.CloseCloudRepo(context.Background()); err != nil {
		t.Fatal(err)
	}
	util.DataDir = "/fixture/replacement"
	if err = captured.guard(context.Background()); err == nil {
		t.Fatal("captured watcher accepted replacement roots")
	}
}
