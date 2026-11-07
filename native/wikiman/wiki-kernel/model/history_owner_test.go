package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/task"
)

func TestAutoHistoryTaskCancellationAndActualJoin(t *testing.T) {
	owner := legacyBackgroundOwnerFixture(t)
	history, err := captureLegacyAutoHistoryOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	group := taskgroup.Open()
	entered, release := make(chan struct{}), make(chan struct{})
	if err = enqueueAutoFileHistory(context.Background(), history, group, func(ctx context.Context, got *legacyAutoHistoryOwner) error {
		if got != history {
			t.Error("history owner changed")
		}
		close(entered)
		<-release
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	execDone := make(chan struct{})
	go func() { task.ExecTaskJob(); close(execDone) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("history task did not enter")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- owner.CloseCloudRepo(context.Background()) }()
	select {
	case err = <-closeDone:
		t.Fatal("workspace owner closed before history writer returned", err)
	case <-time.After(20 * time.Millisecond):
	}
	groupDone := make(chan error, 1)
	go func() { groupDone <- group.Close(context.Background()) }()
	select {
	case err = <-groupDone:
		t.Fatal("history ticker owner drained before writer returned", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err = <-closeDone; err != nil {
		t.Fatal(err)
	}
	if err = <-groupDone; err != nil {
		t.Fatal(err)
	}
	<-execDone
}

func TestAutoHistoryPendingTaskIsUniqueAndCancelledWithOwner(t *testing.T) {
	legacyBackgroundOwnerFixture(t)
	history, err := captureLegacyAutoHistoryOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	group := taskgroup.Open()
	calls := 0
	generate := func(context.Context, *legacyAutoHistoryOwner) error { calls++; return nil }
	if err = enqueueAutoFileHistory(context.Background(), history, group, generate); err != nil {
		t.Fatal(err)
	}
	if err = enqueueAutoFileHistory(context.Background(), history, group, generate); err != nil {
		t.Fatal(err)
	}
	if err = group.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	task.ExecTaskJob()
	if calls != 0 {
		t.Fatal("cancelled pending history task ran", calls)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err = enqueueAutoFileHistory(cancelled, history, taskgroup.Open(), generate); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
