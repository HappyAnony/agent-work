package task

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestContextTaskPendingCancellationAndClosedAdmission(t *testing.T) {
	OpenLegacyTasks()
	defer CloseLegacyTasks(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	h, e := AppendContext(ctx, "test.pending", func(context.Context) error { t.Error("canceled pending handler ran"); return nil })
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	if e = h.Wait(context.Background()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	ExecTaskJob()
	if e = CloseLegacyTasks(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = AppendContext(context.Background(), "late", func(context.Context) error { return nil }); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
}
func TestContextTaskRunningCancellationAndTimeoutWaitActualReturn(t *testing.T) {
	OpenLegacyTasks()
	defer CloseLegacyTasks(context.Background())
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h, e := AppendContext(context.Background(), HistoryGenerateFile, func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		close(canceled)
		<-release
		return ctx.Err()
	})
	if e != nil {
		t.Fatal(e)
	}
	queueLock.Lock()
	taskQueue[0].Timeout = time.Millisecond
	queueLock.Unlock()
	done := make(chan struct{})
	go func() { ExecTaskJob(); close(done) }()
	<-entered
	time.Sleep(5 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("owned serial handler released on timeout")
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if e = CloseLegacyTasks(ctx); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("false join", e)
	}
	<-canceled
	close(release)
	if e = h.Wait(context.Background()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	<-done
	if e = CloseLegacyTasks(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e = AppendContext(context.Background(), HistoryGenerateFile, func(context.Context) error { return nil }); !errors.Is(e, ErrClosed) {
		t.Fatal("history task admitted after close", e)
	}
}
func TestContextTaskPanicCompletesAndQueueContinues(t *testing.T) {
	OpenLegacyTasks()
	defer CloseLegacyTasks(context.Background())
	h, e := AppendContext(context.Background(), "test.panic", func(context.Context) error { panic("synthetic") })
	if e != nil {
		t.Fatal(e)
	}
	ExecTaskJob()
	if e = h.Wait(context.Background()); !errors.Is(e, ErrHandler) {
		t.Fatal(e)
	}
	ran := false
	h, e = AppendContext(context.Background(), "next", func(context.Context) error { ran = true; return nil })
	if e != nil {
		t.Fatal(e)
	}
	ExecTaskJob()
	if e = h.Wait(context.Background()); e != nil || !ran {
		t.Fatal(e)
	}
}

func TestAsyncContextTaskRejectedDispatchCompletesWaiter(t *testing.T) {
	OpenLegacyTasks()
	if err := CloseLegacyTasks(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := &ContextTask{parent: context.Background(), done: make(chan struct{}), fn: func(context.Context) error {
		t.Fatal("rejected async task ran")
		return nil
	}}
	// Insert an owned task after shutdown to model a task popped immediately
	// before its owner stopped. Rejected dispatch must still complete its waiter.
	queueLock.Lock()
	taskQueue = append(taskQueue, &Task{Action: "test.async.rejected", Created: time.Now().Add(-time.Second), Async: true, Owned: h})
	queueLock.Unlock()
	ExecAsyncTaskJob()
	if err := h.Wait(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatal("rejected task waiter was orphaned", err)
	}
}

func TestCancelledCronTickDoesNotPopContextTask(t *testing.T) {
	OpenLegacyTasks()
	defer CloseLegacyTasks(context.Background())
	h, err := AppendContext(context.Background(), "test.cancelled.tick", func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ExecTaskJobContext(ctx)
	queueLock.Lock()
	queued := len(taskQueue)
	queueLock.Unlock()
	if queued != 1 {
		t.Fatal("cancelled cron tick popped queued work", queued)
	}
	ExecTaskJob()
	if err = h.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUniqueContextTaskCoalescesWithoutBorrowingSecondRequest(t *testing.T) {
	OpenLegacyTasks()
	defer CloseLegacyTasks(context.Background())
	first, err := AppendUniqueContext(context.Background(), HistoryDatabaseIndexFull, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	secondCtx, cancel := context.WithCancel(context.Background())
	second, err := AppendUniqueContext(secondCtx, HistoryDatabaseIndexFull, func(context.Context) error {
		t.Fatal("duplicate handler ran")
		return nil
	})
	cancel()
	if second != nil || !errors.Is(err, ErrDuplicate) {
		t.Fatal(second, err)
	}
	ExecTaskJob()
	if err = first.Wait(context.Background()); err != nil {
		t.Fatal("second request canceled the accepted task", err)
	}
}

func TestAsyncContextWithDelayPreservesDelayArgumentsAndCancellation(t *testing.T) {
	OpenLegacyTasks()
	defer CloseLegacyTasks(context.Background())
	ran := make(chan string, 2)
	first, err := AppendAsyncContextWithDelay(context.Background(), ReloadProtyle, 20*time.Millisecond, func(context.Context) error {
		ran <- "first"
		return nil
	}, "root-a")
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, duplicateErr := AppendAsyncContextWithDelay(context.Background(), ReloadProtyle, 20*time.Millisecond, func(context.Context) error {
		t.Fatal("duplicate delayed handler ran")
		return nil
	}, "root-a"); duplicate != nil || !errors.Is(duplicateErr, ErrDuplicate) {
		t.Fatal(duplicate, duplicateErr)
	}
	second, err := AppendAsyncContextWithDelay(context.Background(), ReloadProtyle, 20*time.Millisecond, func(context.Context) error {
		ran <- "second"
		return nil
	}, "root-b")
	if err != nil {
		t.Fatal(err)
	}
	ExecAsyncTaskJob()
	select {
	case value := <-ran:
		t.Fatal("delayed handler ran early", value)
	default:
	}
	second.Cancel()
	time.Sleep(25 * time.Millisecond)
	ExecAsyncTaskJob()
	if err = first.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = second.Wait(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case value := <-ran:
		if value != "first" {
			t.Fatal("unexpected delayed handler", value)
		}
	default:
		t.Fatal("accepted delayed handler did not run")
	}
}
