package model

import (
	"context"
	"errors"
	"github.com/siyuan-note/eventbus"
	"github.com/siyuan-note/logging"
	"sync"
	"wiki-kernel/cache"
	"wiki-kernel/internal/eventscope"
	"wiki-kernel/internal/taskgroup"
	"wiki-kernel/sql"
	"wiki-kernel/task"
)

var legacyEventBridge = eventscope.NewBridge(eventbus.Subscribe)
var legacyEventScope *eventscope.Scope
var legacyEventRegistrationError error
var legacyRuntimeStarted bool
var legacyRuntimeLock sync.Mutex
var legacyRuntimeClosed bool
var legacyBootTasks *taskgroup.Group

// StartLegacyRuntime explicitly enables the transitional GUI runtime's process
// subscriptions and transaction worker. Importing model no longer does either.
// This is NOT a multi-workspace adapter: Conf, DBs and event callbacks are still
// global and must not be imported by the contextual Server or CLI service path.
func StartLegacyRuntime() error {
	legacyRuntimeLock.Lock()
	defer legacyRuntimeLock.Unlock()
	if legacyRuntimeClosed {
		return errors.New("legacy_runtime_closed")
	}
	if legacyRuntimeStarted {
		return nil
	}
	if err := cache.OpenLegacyIAL(); err != nil {
		return err
	}
	if err := sql.OpenLegacyCaches(); err != nil {
		cache.CloseLegacyIAL()
		return err
	}
	legacyEventScope = legacyEventBridge.Open()
	legacyEventRegistrationError = nil
	sql.SetLegacySQLEventOwnerProvider(captureLegacySQLEventToken)
	subscribeSQLEvents()
	subscribeSQLHistoryEvents()
	subscribeSQLAssetContentEvents()
	subscribeRepoEvents()
	if legacyEventRegistrationError != nil {
		startErr := legacyEventRegistrationError
		closeErr := legacyEventScope.Close(context.Background())
		legacyEventScope = nil
		legacyEventRegistrationError = nil
		sql.SetLegacySQLEventOwnerProvider(nil)
		cache.CloseLegacyIAL()
		sql.CloseLegacyCaches()
		return errors.Join(startErr, closeErr)
	}
	StartTransactionQueue()
	task.OpenLegacyTasks()
	legacyBootTasks = taskgroup.Open()
	legacyRuntimeStarted = true
	return nil
}

// StartLegacyBootTask admits process-scoped compatibility initialization work.
// Unlike workspace work, these callbacks belong to the legacy runtime itself;
// shutdown cancels them and waits for their actual return before closing DBs.
func StartLegacyBootTask(fn func(context.Context)) bool {
	legacyRuntimeLock.Lock()
	defer legacyRuntimeLock.Unlock()
	if !legacyRuntimeStarted || legacyRuntimeClosed || legacyBootTasks == nil || fn == nil {
		return false
	}
	return legacyBootTasks.Go(func(ctx context.Context) {
		defer logging.Recover()
		fn(ctx)
	})
}

func closeLegacyBootTasks(ctx context.Context) error {
	if legacyBootTasks == nil {
		return nil
	}
	legacyBootTasks.Stop()
	return legacyBootTasks.Wait(ctx)
}

// Called only while StartLegacyRuntime holds its owner lock. Original callback
// bodies and signatures are unchanged; every registration failure is surfaced.
func subscribeLegacyEvent(topic string, fn interface{}) {
	if legacyEventScope == nil {
		legacyEventRegistrationError = errors.Join(legacyEventRegistrationError, errors.New("legacy_event_owner_missing"))
		return
	}
	legacyEventRegistrationError = errors.Join(legacyEventRegistrationError, legacyEventScope.Subscribe(topic, fn))
}

// StopLegacySubscriptions rejects late callbacks, removes captured model
// references, and waits for callbacks that already entered before DB teardown.
func StopLegacySubscriptions(ctx context.Context) error {
	legacyRuntimeLock.Lock()
	scope := legacyEventScope
	legacyRuntimeLock.Unlock()
	if scope == nil {
		return nil
	}
	if err := scope.Close(ctx); err != nil {
		return err
	}
	sql.SetLegacySQLEventOwnerProvider(nil)
	return nil
}

// The compat owner calls this after its own direct readers/DBs are drained.
// A cancelled transaction drain leaves live caches intact for a safe retry.
func CloseLegacyRuntime(ctx context.Context) error {
	legacyRuntimeLock.Lock()
	defer legacyRuntimeLock.Unlock()
	legacyRuntimeClosed = true
	if err := StopLegacyServices(ctx); err != nil {
		return err
	}
	if err := closeLegacyBootTasks(ctx); err != nil {
		return err
	}
	if err := task.CloseLegacyTasks(ctx); err != nil {
		return err
	}
	if legacyEventScope != nil {
		if err := legacyEventScope.Close(ctx); err != nil {
			return err
		}
	}
	if err := CloseTransactionQueue(ctx); err != nil {
		return err
	}
	sql.SetLegacySQLEventOwnerProvider(nil)
	cache.CloseLegacyIAL()
	sql.CloseLegacyCaches()
	return nil
}
