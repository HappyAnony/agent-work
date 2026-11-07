package model

import (
	"context"
	"errors"
	"testing"
)

func TestTransactionBatchRejectsMalformedBeforeMutation(t *testing.T) {
	for _, kind := range []string{"nil-batch", "nil-transaction", "nil-do", "nil-undo"} {
		t.Run(kind, func(t *testing.T) {
			first := &Transaction{DoOperations: []*Operation{{Action: "create"}}}
			batch := []*Transaction{first}
			switch kind {
			case "nil-transaction":
				batch = append(batch, nil)
			case "nil-do":
				batch = append(batch, &Transaction{DoOperations: []*Operation{nil}})
			case "nil-undo":
				batch = append(batch, &Transaction{UndoOperations: []*Operation{nil}})
			}
			var err error
			if kind == "nil-batch" {
				err = PerformTransactionsContext(context.Background(), nil)
			} else {
				err = PerformTransactionsContext(context.Background(), &batch)
			}
			if err == nil {
				t.Fatal("malformed batch accepted")
			}
			want := "transaction_operation_invalid"
			if kind == "nil-batch" {
				want = "transaction_batch_unbound"
			}
			if kind == "nil-transaction" {
				want = "transaction_batch_invalid"
			}
			if err.Error() != want {
				t.Fatalf("batch reached dependencies before validation: %v", err)
			}
			if first.environment != nil || first.m != nil {
				t.Fatal("earlier transaction mutated before full validation")
			}
		})
	}
}

func TestTransactionBatchValidationPreservesEmptyAndCancellation(t *testing.T) {
	if err := ValidateTransactionsContext(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransactionsContext(context.Background(), []*Transaction{{}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateTransactionsContext(ctx, []*Transaction{nil}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
}
