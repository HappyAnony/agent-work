package model

import (
	"context"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docquery"
)

type blockFoldEnvironment struct {
	services docquery.FoldServices
}

func legacyBlockFoldEnvironment() *blockFoldEnvironment {
	return &blockFoldEnvironment{services: docquery.FoldServices{
		Load: func(_ context.Context, id string) (*parse.Tree, error) {
			// Original public helpers return empty/false on a missing source.
			// Keep that policy in the explicit compatibility adapter only.
			t, _ := LoadTreeByBlockID(id)
			return t, nil
		},
	}}
}
func (tx *Transaction) blockFold(q docquery.FoldRequest) (docquery.FoldResult, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.blockFold == nil {
			return docquery.FoldResult{}, docquery.ErrFoldDependency
		}
		return docquery.Fold(tx.environment.ctx, q, tx.environment.blockFold.services)
	}
	return docquery.Fold(context.Background(), q, legacyBlockFoldEnvironment().services)
}
