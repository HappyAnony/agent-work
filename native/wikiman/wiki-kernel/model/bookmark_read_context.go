package model

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/treenode"
)

func bookmarkReadWithOwner(ctx context.Context, c *legacyCommitContext, tx *Transaction) *bookmarkReadEnvironment {
	r := c.r
	return &bookmarkReadEnvironment{ctx: ctx, caseSensitive: r.caseSensitive, ownerGuard: func() error { return r.guard(ctx) },
		wait: func() error {
			if err := r.guard(ctx); err != nil {
				return err
			}
			if tx != nil {
				return nil
			}
			return FlushTxQueueContext(ctx)
		},
		flush: func() error {
			if err := r.guard(ctx); err != nil {
				return err
			}
			return r.queue.Flush(ctx)
		},
		labels: func(ctx context.Context) ([]string, error) {
			if err := r.guard(ctx); err != nil {
				return nil, err
			}
			return indexdb.BookmarkLabels(ctx, r.main.DB)
		},
		services: blockview.BookmarkServices{Legacy: true,
			Query: func(ctx context.Context) ([]indexdb.BlockRecord, error) {
				if err := r.guard(ctx); err != nil {
					return nil, err
				}
				return indexdb.BookmarkBlocks(ctx, r.main.DB)
			},
			Parser: func(ctx context.Context) (*lute.Lute, error) {
				if err := r.guard(ctx); err != nil {
					return nil, err
				}
				return c.parser(), nil
			},
			Node: func(ctx context.Context, id string) (*ast.Node, error) {
				if err := r.guard(ctx); err != nil {
					return nil, err
				}
				if tx != nil {
					tree, err := tx.loadTree(id)
					if err != nil {
						return nil, err
					}
					return treenode.GetNodeInTree(tree, id), nil
				}
				bt, err := r.referenceLookup(ctx, id)
				if err != nil {
					return nil, err
				}
				if bt == nil {
					return nil, ErrBlockNotFound
				}
				tree, err := r.loadRepairTree(ctx, bt.BoxID, bt.Path, c.parser())
				if err != nil {
					return nil, err
				}
				return treenode.GetNodeInTree(tree, id), nil
			},
			AttributeViewName: func(ctx context.Context, id string) (string, error) {
				if !ast.IsNodeIDPattern(id) {
					return "", errors.New("bookmark_av_id_invalid")
				}
				scope := c.avContext(ctx)
				scope.readOnly = true
				raw, err := scope.source("storage/av/" + id + ".json")
				if err != nil {
					return "", err
				}
				var value struct {
					Name string `json:"name"`
				}
				err = json.Unmarshal(raw, &value)
				return value.Name, err
			},
		},
	}
}

func BuildBookmarkContext(ctx context.Context) (*Bookmarks, error) {
	if ctx == nil {
		return nil, errors.New("bookmark_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o, err := captureLegacyAnchorContext(ctx)
	if err != nil {
		return nil, err
	}
	result, err := buildBookmarksWithEnvironment(bookmarkReadWithOwner(ctx, o.commit, nil))
	if err != nil {
		return nil, err
	}
	if err = o.r.guard(ctx); err != nil {
		return nil, err
	}
	return &result, nil
}

func BookmarkLabelsContext(ctx context.Context) ([]string, error) {
	if ctx == nil {
		return nil, errors.New("bookmark_context_unbound")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o, err := captureLegacyAnchorContext(ctx)
	if err != nil {
		return nil, err
	}
	result, err := bookmarkLabelsWithEnvironment(bookmarkReadWithOwner(ctx, o.commit, nil))
	if err != nil {
		return nil, err
	}
	if err = o.r.guard(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
