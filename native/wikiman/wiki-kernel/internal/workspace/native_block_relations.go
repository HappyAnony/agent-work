package workspace

import (
	"context"
	"github.com/88250/lute/parse"
	"slices"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeBlockTreeInfos(ctx context.Context, ids []string) (map[string]*docquery.BlockTreeInfo, error) {
	r, e := l.NativeBlockRelations(ctx, docquery.RelationRequest{Mode: "infos", IDs: ids})
	return r.Infos, e
}
func (l *Lease) NativeBlockSiblingID(ctx context.Context, id string) (docquery.RelationIDs, error) {
	r, e := l.NativeBlockRelations(ctx, docquery.RelationRequest{Mode: "sibling", ID: id})
	return r.IDs, e
}
func (l *Lease) NativeBlockRelevantIDs(ctx context.Context, id string) (docquery.RelationIDs, error) {
	r, e := l.NativeBlockRelations(ctx, docquery.RelationRequest{Mode: "relevant", ID: id})
	return r.IDs, e
}

// One source admission/index gate reads this owner's actual registry and trees.
// No main flush, AST annotations, default provider, SQL row substitute or repair.
func (l *Lease) NativeBlockRelations(ctx context.Context, q docquery.RelationRequest) (ret docquery.RelationResult, err error) {
	q.IDs = slices.Clone(q.IDs)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		var e error
		ret, e = docquery.Relations(ctx, q, docquery.RelationServices{
			Trees: func(ctx context.Context, ids []string) (map[string]*parse.Tree, error) {
				return r.documentInfoTrees(ctx, w, ids)
			},
			Load: func(ctx context.Context, id string) (*parse.Tree, error) {
				trees, e := r.documentInfoTrees(ctx, w, []string{id})
				if e != nil {
					return nil, e
				}
				return trees[id], nil
			}, BlockMissing: ErrDocument,
		})
		return e
	})
	if err != nil {
		return docquery.RelationResult{}, err
	}
	return
}
