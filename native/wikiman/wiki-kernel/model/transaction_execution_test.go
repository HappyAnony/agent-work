//go:build fts5

package model

import (
	"context"
	"errors"
	"testing"
	"time"
	"wiki-kernel/internal/flashcard"
)

func TestTransactionExecutionWaitIncludesQueuedWork(t *testing.T) {
	commitFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	isolatedConflictQueue(t, func(tx *Transaction) { close(entered); <-release; flushTx(tx) })
	tx := &Transaction{}
	batch := []*Transaction{tx}
	if e := PerformTransactionsContext(context.Background(), &batch); e != nil {
		t.Fatal(e)
	}
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	e := tx.WaitForCommitContext(ctx)
	cancel()
	close(release)
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("queued work returned early: %v", e)
	}
	if e = tx.WaitForCommitContext(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = PerformTransactionsContext(context.Background(), &batch); e == nil {
		t.Fatal("admitted transaction reused")
	}
}

func TestTransactionExecutionReportsFailureAndCommittedPublicationError(t *testing.T) {
	for _, kind := range []string{"rollback", "after-commit", "panic"} {
		t.Run(kind, func(t *testing.T) {
			_, env, _, _, _ := commitFixture(t)
			want := errors.New("execution failed")
			env.reportFailure = func(*TxErr, *Transaction) {}
			env.finish = func(*Transaction) error {
				if kind == "panic" {
					panic("worker failure")
				}
				if kind == "rollback" {
					return want
				}
				return nil
			}
			env.afterCommit = func(context.Context, []flashcard.AttributeEffect, []string) error { return want }
			tx := &Transaction{environment: env, execution: &transactionExecution{done: make(chan struct{})}, DoOperations: []*Operation{{Action: ""}}}
			flushTx(tx)
			if e := tx.WaitForCommitContext(context.Background()); e == nil {
				t.Fatal("execution failure reported as success")
			} else if kind == "after-commit" && !errors.Is(e, errTransactionAfterCommit) {
				t.Fatalf("committed marker lost: %v", e)
			}
			if kind == "after-commit" && tx.state.Load() != 2 {
				t.Fatal("completed commit rolled back")
			}
		})
	}
}

func TestTransactionBatchRejectsDuplicatePointersBeforeMutation(t *testing.T) {
	tx := &Transaction{}
	if e := ValidateTransactionsContext(context.Background(), []*Transaction{tx, tx}); e == nil {
		t.Fatal("duplicate pointer accepted")
	}
	if tx.execution != nil || tx.environment != nil {
		t.Fatal("validation mutated transaction")
	}
}

func TestTransactionExecutionWaitIncludesPostCommitPublication(t *testing.T) {
	_, env, _, _, _ := commitFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	env.finish = func(*Transaction) error { return nil }
	env.afterCommit = func(context.Context, []flashcard.AttributeEffect, []string) error {
		close(entered)
		<-release
		return nil
	}
	tx := &Transaction{environment: env, execution: &transactionExecution{done: make(chan struct{})}, DoOperations: []*Operation{{Action: ""}}}
	go flushTx(tx)
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	e := tx.WaitForCommitContext(ctx)
	cancel()
	close(release)
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("publication returned early: %v", e)
	}
	if e = tx.WaitForCommitContext(context.Background()); e != nil {
		t.Fatal(e)
	}
}
