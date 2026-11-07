package model

import (
	"context"
	"errors"
	"github.com/siyuan-note/eventbus"
	"testing"
	"time"

	"wiki-kernel/cache"
	"wiki-kernel/internal/eventscope"
	"wiki-kernel/internal/txqueue"
)

// Empty original transactions exercise the real flushTx/performTx chain without
// opening a personal workspace, configuring providers or initializing databases.
func TestLegacyTransactionQueueLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if legacyTxQueue != nil {
		t.Fatal("model import started a worker")
	}
	if err := PerformTransactions(&[]*Transaction{{}}); !errors.Is(err, txqueue.ErrClosed) {
		t.Fatal(err)
	}
	StartTransactionQueue()
	first := legacyTxQueue
	StartTransactionQueue()
	if legacyTxQueue != first {
		t.Fatal("duplicate transaction worker")
	}
	defer func() { _ = first.Close(ctx); legacyTxQueue = nil }()
	if err := PerformTransactions(&[]*Transaction{{}, {}}); err != nil {
		t.Fatal(err)
	}
	FlushTxQueue()
	if err := CloseTransactionQueue(ctx); err != nil {
		t.Fatal(err)
	}
	if err := PerformTransactions(&[]*Transaction{{}}); !errors.Is(err, txqueue.ErrClosed) {
		t.Fatal(err)
	}
	if err := CloseTransactionQueue(ctx); err != nil {
		t.Fatal(err)
	}
	StartTransactionQueue()
	if legacyTxQueue != first {
		t.Fatal("closed runtime silently reopened")
	}
}

func TestLegacyRuntimeExplicitCachesAndClosedAdmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer func() {
		_ = CloseLegacyRuntime(ctx)
		legacyTxQueue = nil
		legacyRuntimeClosed = false
		legacyRuntimeStarted = false
	}()
	if err := StartLegacyRuntime(); err != nil {
		t.Fatal(err)
	}
	first := legacyTxQueue
	if err := StartLegacyRuntime(); err != nil || first != legacyTxQueue {
		t.Fatal("duplicate runtime", err)
	}
	bootCanceled := make(chan struct{})
	if !StartLegacyBootTask(func(ctx context.Context) {
		<-ctx.Done()
		close(bootCanceled)
	}) {
		t.Fatal("runtime boot owner unavailable")
	}
	calls := 0
	subscribeLegacyEvent("test.r02.owned-callback", func() { calls++ })
	if legacyEventRegistrationError != nil {
		t.Fatal(legacyEventRegistrationError)
	}
	eventbus.Publish("test.r02.owned-callback")
	if err := CloseLegacyRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-bootCanceled:
	default:
		t.Fatal("runtime close did not join boot task")
	}
	if err := StartLegacyRuntime(); err == nil {
		t.Fatal("closed runtime recreated resources")
	}
	if cache.GetDocIAL("late") != nil {
		t.Fatal("closed IAL cache usable")
	}
	eventbus.Publish("test.r02.owned-callback")
	if calls != 1 {
		t.Fatal("closed model callback executed", calls)
	}
}

func TestLegacyRuntimeRegistrationFailureRollsBackAndCanRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	originalBridge := legacyEventBridge
	injected := errors.New("injected event registration failure")
	defer func() {
		_ = CloseLegacyRuntime(ctx)
		legacyEventBridge = originalBridge
		legacyEventScope = nil
		legacyEventRegistrationError = nil
		legacyTxQueue = nil
		legacyRuntimeClosed = false
		legacyRuntimeStarted = false
	}()
	legacyRuntimeClosed = false
	legacyRuntimeStarted = false
	legacyEventScope = nil
	legacyEventBridge = eventscope.NewBridge(func(string, interface{}) error { return injected })
	if err := StartLegacyRuntime(); !errors.Is(err, injected) {
		t.Fatal(err)
	}
	if legacyEventScope != nil || legacyRuntimeStarted || cache.GetDocIAL("failed-start") != nil {
		t.Fatal("failed runtime retained owned state")
	}
	legacyEventBridge = originalBridge
	if err := StartLegacyRuntime(); err != nil {
		t.Fatal("retry after rollback", err)
	}
	if !legacyRuntimeStarted || legacyEventScope == nil {
		t.Fatal("retry did not publish runtime")
	}
}
