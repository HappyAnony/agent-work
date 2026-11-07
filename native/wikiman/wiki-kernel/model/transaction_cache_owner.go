package model

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/cache"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
)

func bindTransactionCachePorts(env *transactionEnvironment, c *legacyCommitContext) {
	env.stage = func(tree *parse.Tree) error {
		if err := c.r.guard(env.ctx); err != nil {
			return err
		}
		if err := c.r.registry.Upsert(env.ctx, tree); err != nil {
			return err
		}
		return c.r.guard(env.ctx)
	}
	env.cacheIAL = cache.CaptureBlockIALPublisher()
	publish := sql.CaptureReferencePublisher()
	env.updateCacheRef = func(ctx context.Context, tree *parse.Tree, node *ast.Node) error {
		if err := c.r.guard(ctx); err != nil {
			return err
		}
		ref, err := indexdb.ProjectReference(ctx, tree, node, c.parser(), func(ctx context.Context, id string) (indexdb.BlockLocation, bool, error) {
			rows, err := c.r.registry.Query(ctx, indexdb.BlockTreeFilter{Kind: "id", Value: id})
			if err != nil || len(rows) == 0 {
				return indexdb.BlockLocation{}, false, err
			}
			return indexdb.BlockLocation{ParentID: rows[0].ParentID, RootID: rows[0].RootID, Path: rows[0].Path}, true, nil
		})
		if err != nil {
			return err
		}
		if err = c.r.guard(ctx); err != nil {
			return err
		}
		return publish(ctx, ref)
	}
}
