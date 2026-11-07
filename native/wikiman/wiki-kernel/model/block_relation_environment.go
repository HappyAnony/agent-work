package model

import (
	"context"
	"github.com/88250/lute/parse"
	"wiki-kernel/filesys"
	"wiki-kernel/internal/docquery"
)

type blockRelationEnvironment struct {
	ctx      context.Context
	services docquery.RelationServices
}

func legacyBlockRelationEnvironment() *blockRelationEnvironment {
	return &blockRelationEnvironment{ctx: context.Background(), services: docquery.RelationServices{
		Trees: func(_ context.Context, ids []string) (map[string]*parse.Tree, error) {
			return filesys.LoadTrees(ids), nil
		},
		Load: func(_ context.Context, id string) (*parse.Tree, error) { return LoadTreeByBlockID(id) }, BlockMissing: ErrBlockNotFound,
	}}
}
func (tx *Transaction) blockRelations(q docquery.RelationRequest) (docquery.RelationResult, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		e := tx.environment.blockRelations
		if e == nil {
			return docquery.RelationResult{}, docquery.ErrRelationDependency
		}
		return docquery.Relations(tx.environment.ctx, q, e.services)
	}
	e := legacyBlockRelationEnvironment()
	return docquery.Relations(e.ctx, q, e.services)
}
