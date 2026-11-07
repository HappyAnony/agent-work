package workspace

import (
	"context"
	"errors"
	"io/fs"
	"wiki-kernel/internal/indexdb"
)

func copyNativeSearchRequest(request indexdb.BlockSearchRequest) indexdb.BlockSearchRequest {
	// Snapshot mutable request values before borrowing the gate.
	request.Boxes = append([]string(nil), request.Boxes...)
	request.Paths = append([]string(nil), request.Paths...)
	if request.Types != nil {
		copy := map[string]bool{}
		for k, v := range request.Types {
			copy[k] = v
		}
		request.Types = copy
	}
	return request
}
func (l *Lease) SearchNativeBlocks(ctx context.Context, request indexdb.BlockSearchRequest) (ret indexdb.BlockSearchResult, err error) {
	request = copyNativeSearchRequest(request)
	err = l.WithNativeIndexes(ctx, func(*indexdb.Bundle) error {
		var e error
		ret, e = l.workspace.resources.searchNativeBlocks(ctx, l.workspace, request)
		return e
	})
	return
}
func (r *resources) searchNativeBlocks(ctx context.Context, w *WorkspaceContext, request indexdb.BlockSearchRequest) (indexdb.BlockSearchResult, error) {
	return r.searchNativeBlocksMode(ctx, w, request, false)
}
func (r *resources) searchNativeBlocksMode(ctx context.Context, w *WorkspaceContext, request indexdb.BlockSearchRequest, all bool) (ret indexdb.BlockSearchResult, err error) {
	bundle := r.native
	if r.configuration.Search == nil {
		return ret, indexdb.ErrProjection
	}
	db, e := bundle.Database(indexdb.Main)
	if e != nil {
		return ret, e
	}
	raw, e := readNativeFile(ctx, w, ".wiki/searchignore", 1<<20)
	if e != nil && !errors.Is(e, fs.ErrNotExist) {
		return ret, e
	}
	request.Ignore = indexdb.SearchIgnoreLines(raw)

	if all {
		ret, e = indexdb.SearchAllBlocks(ctx, db, *r.configuration.Search, request)
	} else {
		ret, e = indexdb.SearchBlocks(ctx, db, *r.configuration.Search, request)
	}
	return ret, e
}
