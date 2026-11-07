// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/tagquery"
	"wiki-kernel/sql"
)

type tagNavigationEnvironment struct {
	ctx           context.Context
	limit         int
	caseSensitive bool
	services      tagquery.NavigationServices
}

func legacyTagNavigationEnvironment() *tagNavigationEnvironment {
	return &tagNavigationEnvironment{ctx: context.Background(), limit: Conf.Search.Limit, caseSensitive: Conf.Search.CaseSensitive, services: tagquery.NavigationServices{
		Spans: func(_ context.Context, keyword string, limit int) (ret []indexdb.SpanRecord, err error) {
			for _, row := range sql.QueryTagSpansByKeyword(keyword, limit) {
				if row != nil {
					ret = append(ret, indexdb.SpanRecord(*row))
				}
			}
			return
		},
		Blocks: legacySearchViewServices().Blocks,
	}}
}

func tagBlocksWithEnvironment(keyword string, e *tagNavigationEnvironment) (map[string]TagBlocks, error) {
	if e == nil {
		return nil, errors.New("tag_navigation_dependencies_unbound")
	}
	rows, err := tagquery.Navigate(e.ctx, keyword, e.limit, e.caseSensitive, e.services)
	if err != nil {
		return nil, err
	}
	ret := map[string]TagBlocks{}
	for label, blocks := range rows {
		ret[label] = TagBlocks(blocks)
	}
	return ret, nil
}

func (tx *Transaction) tagBlocksByKeyword(keyword string) (map[string]TagBlocks, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.tagNavigation == nil {
			return nil, errors.New("tag_navigation_dependencies_unbound")
		}
		e := *tx.environment.tagNavigation
		e.ctx = tx.environment.ctx
		return tagBlocksWithEnvironment(keyword, &e)
	}
	return tagBlocksWithEnvironment(keyword, legacyTagNavigationEnvironment())
}
