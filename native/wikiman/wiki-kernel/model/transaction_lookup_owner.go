package model

import (
	"context"
	"errors"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/treenode"
)

func (env *transactionEnvironment) readGuard() error {
	if env == nil || env.ctx == nil {
		return errors.New("transaction_read_context_unbound")
	}
	if err := env.ctx.Err(); err != nil {
		return err
	}
	if env.commitContext != nil {
		return env.commitContext.r.guard(env.ctx)
	}
	return nil
}

func (env *transactionEnvironment) lookupBlock(id string) (*treenode.BlockTree, error) {
	if err := env.readGuard(); err != nil {
		return nil, err
	}
	if env.lookupChecked != nil {
		return env.lookupChecked(env.ctx, id)
	}
	block := env.lookup(id)
	if err := env.readGuard(); err != nil {
		return nil, err
	}
	return block, nil
}

func bindTransactionLookup(env *transactionEnvironment, c *legacyCommitContext) {
	env.lookupChecked = func(ctx context.Context, id string) (*treenode.BlockTree, error) {
		if err := c.r.guard(ctx); err != nil {
			return nil, err
		}
		rows, err := c.r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
		if err != nil {
			return nil, err
		}
		if err = c.r.guard(ctx); err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, nil
		}
		block := treenode.BlockTree(rows[0])
		return &block, nil
	}
}
