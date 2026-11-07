package workspace

import (
	"context"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexdb"
)

func (l *Lease) NativeUnfoldedParentID(ctx context.Context, id string) (string, error) {
	r, e := l.NativeBlockFold(ctx, docquery.FoldRequest{Mode: "unfolded-parent", ID: id})
	return r.ParentID, e
}
func (l *Lease) NativeIsBlockFolded(ctx context.Context, id string) (bool, bool, error) {
	r, e := l.NativeBlockFold(ctx, docquery.FoldRequest{Mode: "state", ID: id})
	return r.IsFolded, r.IsRoot, e
}
func (l *Lease) NativeBlockFold(ctx context.Context, q docquery.FoldRequest) (ret docquery.FoldResult, err error) {
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		// Request-local snapshots only; no process cache or background work.
		loaded := map[string]*parse.Tree{}
		var e error
		ret, e = docquery.Fold(ctx, q, docquery.FoldServices{Load: func(ctx context.Context, id string) (*parse.Tree, error) {
			if t, ok := loaded[id]; ok {
				return t, ctx.Err()
			}
			trees, e := r.documentInfoTrees(ctx, w, []string{id})
			if e != nil {
				return nil, e
			}
			loaded[id] = trees[id]
			return trees[id], nil
		}})
		return e
	})
	if err != nil {
		return docquery.FoldResult{}, err
	}
	return
}
