package model

import (
	"context"
	"errors"
	"testing"
)

func TestPerformTransactionsContextRejectsCanceledAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx := &Transaction{DoOperations: []*Operation{{Action: "create"}}}
	err := PerformTransactionsContext(ctx, &[]*Transaction{tx})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("admission error = %v, want context canceled", err)
	}
	if tx.environment != nil || tx.m != nil {
		t.Fatal("canceled admission mutated transaction")
	}
}

func TestTransactionWaitForCommitContextCancels(t *testing.T) {
	tx := &Transaction{}
	tx.state.Store(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tx.WaitForCommitContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want context canceled", err)
	}

	tx.state.Store(2)
	if err := tx.WaitForCommitContext(context.Background()); err != nil {
		t.Fatalf("completed wait error = %v", err)
	}
	if err := tx.WaitForCommitContext(nil); err == nil || err.Error() != "transaction_context_unbound" {
		t.Fatalf("nil context error = %v", err)
	}
}

func TestLegacyTransactionEnvironmentKeepsAdmissionContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	env := legacyTransactionEnvironmentContext(ctx)
	cancel()
	if err := env.validate(nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("environment error = %v, want context canceled", err)
	}
}

func TestRequestHandoffHelpersRejectCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GetAssetAbsPathContext(ctx, "assets/missing.png"); !errors.Is(err, context.Canceled) {
		t.Fatalf("asset path error = %v", err)
	}

	legacyTxQueueLock.Lock()
	previous := legacyTxQueue
	legacyTxQueue = nil
	legacyTxQueueLock.Unlock()
	t.Cleanup(func() {
		legacyTxQueueLock.Lock()
		legacyTxQueue = previous
		legacyTxQueueLock.Unlock()
	})
	if err := FlushTxQueueContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("queue flush error = %v", err)
	}
}
