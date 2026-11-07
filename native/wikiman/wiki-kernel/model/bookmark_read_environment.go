// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/siyuan-note/logging"
	"wiki-kernel/av"
	"wiki-kernel/internal/blockview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/treenode"
)

type bookmarkReadEnvironment struct {
	ownerGuard    func() error
	ctx           context.Context
	caseSensitive bool
	wait, flush   func() error
	labels        func(context.Context) ([]string, error)
	services      blockview.BookmarkServices
}

func legacyBookmarkReadEnvironment() *bookmarkReadEnvironment {
	return &bookmarkReadEnvironment{ctx: context.Background(), caseSensitive: Conf.Search.CaseSensitive,
		wait: func() error { FlushTxQueue(); return nil }, flush: func() error { sql.FlushQueue(); return nil },
		labels: func(context.Context) ([]string, error) { return sql.QueryBookmarkLabels(), nil },
		services: blockview.BookmarkServices{Legacy: true,
			Query: func(context.Context) (ret []indexdb.BlockRecord, err error) {
				for _, b := range sql.QueryBookmarkBlocks() {
					if b != nil {
						ret = append(ret, indexdb.BlockRecord(*b))
					}
				}
				return
			},
			Parser: func(context.Context) (*lute.Lute, error) { return NewLute(), nil },
			Node: func(_ context.Context, id string) (*ast.Node, error) {
				tree, err := LoadTreeByBlockID(id)
				if err != nil {
					return nil, err
				}
				return treenode.GetNodeInTree(tree, id), nil
			},
			AttributeViewName: func(_ context.Context, id string) (string, error) { return av.GetAttributeViewName(id) },
			Warn:              func(id string, err error) { logging.LogErrorf("parse block [%s] failed: %s", id, err) },
		},
	}
}
func buildBookmarksWithEnvironment(e *bookmarkReadEnvironment) (blockview.Bookmarks, error) {
	if e == nil || e.ctx == nil || e.wait == nil || e.flush == nil {
		return nil, errors.New("bookmark_read_dependencies_unbound")
	}
	if err := blockview.ValidateBookmarkServices(e.ctx, e.services); err != nil {
		return nil, err
	}
	if err := e.ctx.Err(); err != nil {
		return nil, err
	}
	if err := e.wait(); err != nil {
		return nil, err
	}
	if err := e.ctx.Err(); err != nil {
		return nil, err
	}
	if err := e.flush(); err != nil {
		return nil, err
	}
	ret, err := blockview.BookmarksView(e.ctx, e.caseSensitive, e.services)
	if err != nil {
		return nil, err
	}
	if e.ownerGuard != nil {
		if err = e.ownerGuard(); err != nil {
			return nil, err
		}
	}
	return ret, nil
}
func bookmarkLabelsWithEnvironment(e *bookmarkReadEnvironment) ([]string, error) {
	if e == nil || e.ctx == nil || e.labels == nil {
		return nil, errors.New("bookmark_read_dependencies_unbound")
	}
	if err := e.ctx.Err(); err != nil {
		return nil, err
	}
	ret, err := e.labels(e.ctx)
	if err != nil {
		return nil, err
	}
	if err = e.ctx.Err(); err != nil {
		return nil, err
	}
	if e.ownerGuard != nil {
		if err = e.ownerGuard(); err != nil {
			return nil, err
		}
	}
	return append([]string{}, ret...), nil
}
func (tx *Transaction) bookmarkReadEnvironment() *bookmarkReadEnvironment {
	if tx != nil && tx.environment != nil && tx.environment.legacy && tx.environment.commitContext != nil {
		return bookmarkReadWithOwner(tx.environment.ctx, tx.environment.commitContext, tx)
	}
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.bookmarkRead == nil {
			return nil
		}
		e := *tx.environment.bookmarkRead
		e.ctx = tx.environment.ctx
		e.services.Legacy = false
		return &e
	}
	return legacyBookmarkReadEnvironment()
}
func (tx *Transaction) buildBookmarks() (blockview.Bookmarks, error) {
	return buildBookmarksWithEnvironment(tx.bookmarkReadEnvironment())
}
func (tx *Transaction) bookmarkLabels() ([]string, error) {
	return bookmarkLabelsWithEnvironment(tx.bookmarkReadEnvironment())
}
