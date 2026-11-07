package model

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/flashcard"
)

var errTransactionAfterCommit = errors.New("transaction_committed_publication_failed")

func bindTransactionAfterCommit(env *transactionEnvironment, c *legacyCommitContext) {
	reload := sourceEventEnvironmentForOwner(env.ctx, &legacyAnchorContext{r: c.r, commit: c}, nil)
	cacheIAL := env.cacheIAL
	env.afterCommit = func(ctx context.Context, effects []flashcard.AttributeEffect, roots []string) error {
		for _, effect := range effects {
			if err := c.r.guard(ctx); err != nil {
				return err
			}
			published := false
			c.r.owner.publishSyncUI(ctx, func() {
				cacheIAL(effect.ID, effect.New)
				pushBroadcastAttrTransactions(effect.Old, &ast.Node{ID: effect.ID, KramdownIAL: parse.Map2IAL(effect.New), AttributeViewType: effect.AVType})
				published = true
			})
			if !published {
				return errors.New("transaction_after_commit_owner_closed")
			}
		}
		for _, id := range uniqueUpdatedAttributeViews(roots) {
			if err := c.r.guard(ctx); err != nil {
				return err
			}
			if err := reloadProtyleWithEnvironment(id, reload); err != nil {
				return err
			}
		}
		return nil
	}
}
