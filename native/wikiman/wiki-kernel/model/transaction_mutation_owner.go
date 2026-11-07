package model

import (
	"context"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/indexdb"
)

func bindTransactionMutationPorts(env *transactionEnvironment, c *legacyCommitContext) {
	env.deleteBlockTree = func(ctx context.Context, id string) error {
		if err := c.r.guard(ctx); err != nil {
			return err
		}
		_, err := c.r.registry.Remove(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
		if err != nil {
			return err
		}
		return c.r.guard(ctx)
	}
	env.insertAVRegistry = func(ctx context.Context, node *ast.Node) error {
		if err := c.r.guard(ctx); err != nil {
			return err
		}
		// Use the actual FIFO binding; the no-transaction compatibility wrapper
		// would otherwise select the current global owner again.
		local := *env
		local.ctx = ctx
		tx := &Transaction{environment: &local}
		return registerUpdatedAttributeViews(node, legacyAttributeViewUpdateEnvironment(tx))
	}
	readDynamic := env.updateDynamicText
	env.updateDynamicText = func(ctx context.Context, id string) (string, error) {
		if err := c.r.guard(ctx); err != nil {
			return "", err
		}
		value, err := readDynamic(ctx, id)
		if err != nil {
			return "", err
		}
		if err = c.r.guard(ctx); err != nil {
			return "", err
		}
		return value, nil
	}
}
