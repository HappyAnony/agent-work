package workspace

import (
	"context"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

// NativeBookmarkBlocks reads only this lease's index, without an implicit flush
// or a global active workspace. Rename/remove recheck the current tree value.
func (l *Lease) NativeBookmarkBlocks(ctx context.Context) (ret []indexdb.BlockRecord, err error) {
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		ret, e = indexdb.BookmarkBlocks(ctx, db)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

func (l *Lease) NativeBookmarkLabels(ctx context.Context) (ret []string, err error) {
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		db, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		ret, e = indexdb.BookmarkLabels(ctx, db)
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}

// Flush only this owner's accepted main-index work, then borrow a single source
// admission/index gate for query, tree/AV reads and original UI projection.
func (l *Lease) NativeBookmarks(ctx context.Context) (ret blockview.Bookmarks, err error) {
	if err = l.FlushNativeMainQueue(ctx); err != nil {
		return nil, err
	}
	err = l.WithNativeIndexes(ctx, func(bundle *indexdb.Bundle) error {
		r, w := l.workspace.resources, l.workspace
		if r.configuration.Search == nil {
			return indexdb.ErrProjection
		}
		db, e := bundle.Database(indexdb.Main)
		if e != nil {
			return e
		}
		ret, e = blockview.BookmarksView(ctx, r.configuration.Search.CaseSensitive, blockview.BookmarkServices{
			Query: func(ctx context.Context) ([]indexdb.BlockRecord, error) { return indexdb.BookmarkBlocks(ctx, db) },
			Parser: func(context.Context) (*lute.Lute, error) {
				options, e := r.configuredParserOptions(ctx, w)
				if e != nil {
					return nil, e
				}
				return nativeparser.NewEditor(options), nil
			},
			Node: func(ctx context.Context, id string) (*ast.Node, error) {
				if !r.blockTreeReady {
					if e := r.rebuildBlockTree(ctx, w); e != nil {
						return nil, e
					}
				}
				tree, e := r.nativeTreeByBlock(ctx, w, id)
				if e != nil {
					return nil, e
				}
				var found *ast.Node
				ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
					if enter && n.ID == id {
						found = n
						return ast.WalkStop
					}
					return ast.WalkContinue
				})
				return found, nil
			},
			AttributeViewName: func(ctx context.Context, id string) (string, error) {
				name, _, e := r.readNativeAV(ctx, w, id)
				return name, e
			},
		})
		return e
	})
	if err != nil {
		return nil, err
	}
	return
}
