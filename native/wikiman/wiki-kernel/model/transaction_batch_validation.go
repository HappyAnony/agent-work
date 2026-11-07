package model

import (
	"context"
	"errors"
)

// Validate the full batch before assigning owners or admitting its first item.
// This checks structure only; operation semantics remain with the original core.
func ValidateTransactionsContext(ctx context.Context, transactions []*Transaction) error {
	if ctx == nil {
		return errors.New("transaction_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	seen := map[*Transaction]bool{}
	for _, tx := range transactions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if tx == nil {
			return errors.New("transaction_batch_invalid")
		}
		if seen[tx] || tx.execution != nil {
			return errors.New("transaction_already_admitted")
		}
		seen[tx] = true
		for _, ops := range [][]*Operation{tx.DoOperations, tx.UndoOperations} {
			for _, op := range ops {
				if err := ctx.Err(); err != nil {
					return err
				}
				if op == nil {
					return errors.New("transaction_operation_invalid")
				}
			}
		}
	}
	return nil
}
