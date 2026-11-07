package model

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
)

func bindTransactionHeadingEffects(env *transactionEnvironment, c *legacyCommitContext) {
	env.headingEffects = func(ctx context.Context, tree *parse.Tree) error {
		if err := c.r.guard(ctx); err != nil {
			return err
		}
		if err := c.r.queue.Upsert(ctx, tree); err != nil {
			return err
		}
		if err := c.r.guard(ctx); err != nil {
			return err
		}
		c.r.owner.syncRuntime.same.Store(0)
		c.r.owner.planSyncAfter(c.interval)
		return nil
	}
}

func (env *transactionEnvironment) applyHeadingEffects(tree *parse.Tree) error {
	if err := env.ctx.Err(); err != nil {
		return err
	}
	if env.headingEffects != nil {
		return env.headingEffects(env.ctx, tree)
	}
	if env.legacy {
		return errors.New("transaction_heading_owner_unbound")
	}
	// Nonlegacy candidate environments retain their explicit in-memory ports.
	env.sync()
	env.indexTree(tree)
	return env.ctx.Err()
}
