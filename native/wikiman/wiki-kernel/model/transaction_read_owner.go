package model

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/indexdb"
)

func bindTransactionReadAndNoticePorts(env *transactionEnvironment, c *legacyCommitContext) {
	env.referenceCounts = func(ctx context.Context, ids []string) (map[string]int, error) {
		if err := c.r.guard(ctx); err != nil {
			return nil, err
		}
		counts, err := (indexdb.References{DB: c.r.main.DB}).DefinitionCounts(ctx, ids)
		if err != nil {
			return nil, err
		}
		if err = c.r.guard(ctx); err != nil {
			return nil, err
		}
		return counts, nil
	}
	env.avNames = func(ctx context.Context, ids string) (string, error) {
		if err := c.r.guard(ctx); err != nil {
			return "", err
		}
		scope := c.avContext(ctx)
		scope.readOnly = true
		names, err := attributeViewNamesWithServices(ids, func(id string) (string, error) {
			if !ast.IsNodeIDPattern(id) {
				return "", errors.New("transaction_av_id_invalid")
			}
			raw, err := scope.source("storage/av/" + id + ".json")
			if err != nil {
				return "", err
			}
			var a struct {
				Name string `json:"name"`
			}
			if err = json.Unmarshal(raw, &a); err != nil {
				return "", err
			}
			return a.Name, nil
		}, func() string { return c.avUntitled }, false)
		if err != nil {
			return "", err
		}
		if err = c.r.guard(ctx); err != nil {
			return "", err
		}
		return names, nil
	}
	unfold, updated := env.updateUnfold, env.updateEvent
	env.updateUnfold = func(ctx context.Context, h, n *ast.Node) error {
		if err := c.r.guard(ctx); err != nil {
			return err
		}
		return unfold(ctx, h, n)
	}
	env.updateEvent = func(ctx context.Context, txs []*Transaction) error {
		if err := c.r.guard(ctx); err != nil {
			return err
		}
		return updated(ctx, txs)
	}
}
