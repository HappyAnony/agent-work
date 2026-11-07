package model

import (
	"context"
	"errors"
	"wiki-kernel/util"
)

func unboundTransactionReferenceCounts(context.Context, ...string) error {
	return errors.New("transaction_reference_owner_unbound")
}

// The same repair owner is captured at FIFO admission for commit and follow-up
// work. Never recapture the selected workspace when an operation later runs.
func bindTransactionReferenceCounts(env *transactionEnvironment, r *indexRepairContext) {
	schedule := func(ctx context.Context, ids ...string) error {
		if ctx == nil || r == nil || r.owner == nil {
			return errors.New("transaction_reference_owner_unbound")
		}
		if err := r.guard(ctx); err != nil {
			return err
		}
		return r.owner.scheduleReferenceCounts(ctx, r, ids, util.SQLFlushInterval)
	}
	env.insertReferences, env.updateReferences = schedule, schedule
	env.moveReferences, env.deleteReferences = schedule, schedule
}
