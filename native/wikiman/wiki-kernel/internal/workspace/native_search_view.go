package workspace

import (
	"context"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
)

type NativeSearchView struct {
	Blocks        []*blockview.Block `json:"blocks"`
	MatchedBlocks int                `json:"matchedBlockCount"`
	MatchedRoots  int                `json:"matchedRootCount"`
	PageCount     int                `json:"pageCount"`
	DocumentMode  bool               `json:"docMode"`
}

// Query and UI presentation borrow the same source admission and index gate.
// Do not compose two independently admitted lease calls around this projection.
// SearchAllNativeBlockView keeps replacement queries independent of UI paging.
// It adds no route or request flag to the ordinary paginated search endpoint.
func (l *Lease) SearchAllNativeBlockView(ctx context.Context, request indexdb.BlockSearchRequest) (NativeSearchView, error) {
	return l.searchNativeBlockView(ctx, request, 0, true)
}
func (l *Lease) SearchNativeBlockView(ctx context.Context, request indexdb.BlockSearchRequest, groupBy int) (NativeSearchView, error) {
	return l.searchNativeBlockView(ctx, request, groupBy, false)
}
func (l *Lease) searchNativeBlockView(ctx context.Context, request indexdb.BlockSearchRequest, groupBy int, all bool) (ret NativeSearchView, err error) {
	request = copyNativeSearchRequest(request)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		result, e := r.searchNativeBlocksMode(ctx, w, request, all)
		if e != nil {
			return e
		}
		if groupBy == 1 && len(result.Blocks) > 0 && !r.blockTreeReady {
			if e = r.rebuildBlockTree(ctx, w); e != nil {
				return e
			}
		}
		blocks, e := blockview.FullText(ctx, result, groupBy, request.OrderBy, r.configuration.Search.CaseSensitive, blockview.Services{Strict: true,
			Blocks: func(ctx context.Context, ids []string) (ret []*indexdb.BlockRecord, e error) {
				for _, id := range ids {
					b, ok, e := r.nativeBlock(ctx, id)
					if e != nil {
						return nil, e
					}
					if !ok {
						return nil, ErrDocument
					}
					ret = append(ret, &b)
				}
				return ret, nil
			},
			Tree: func(ctx context.Context, id string) (*parse.Tree, error) { return r.nativeTreeByBlock(ctx, w, id) },
			ReferenceCounts: func(ctx context.Context, ids []string) (map[string]int, error) {
				db, e := r.native.Database(indexdb.Main)
				if e != nil {
					return nil, e
				}
				return (indexdb.References{DB: db}).DefinitionCounts(ctx, ids)
			},
		})
		if e != nil {
			return e
		}
		ret = NativeSearchView{Blocks: blocks, MatchedBlocks: result.MatchedBlocks, MatchedRoots: result.MatchedRoots, PageCount: result.PageCount, DocumentMode: result.DocumentMode}
		return nil
	})
	if err != nil {
		return NativeSearchView{}, err
	}
	return
}
