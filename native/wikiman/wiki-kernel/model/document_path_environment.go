// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docquery"
)

type documentPathEnvironment struct {
	ctx      context.Context
	services docquery.PathServices
}

func pathLocation(t *parse.Tree) docquery.Location {
	return docquery.Location{Path: t.Path, Box: t.Box, HPath: t.HPath}
}
func documentPathsWithEnvironment(q docquery.PathRequest, e *documentPathEnvironment) (docquery.PathResult, error) {
	if e == nil {
		return docquery.PathResult{}, errors.New("document_path_dependencies_unbound")
	}
	return docquery.Paths(e.ctx, q, e.services)
}
func (tx *Transaction) documentPaths(q docquery.PathRequest) (docquery.PathResult, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.documentPaths == nil {
			return docquery.PathResult{}, errors.New("document_path_dependencies_unbound")
		}
		e := *tx.environment.documentPaths
		e.ctx = tx.environment.ctx
		return documentPathsWithEnvironment(q, &e)
	}
	ctx := context.Background()
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil {
		ctx = tx.environment.ctx
	}
	return DocumentPathsContext(ctx, q)
}
