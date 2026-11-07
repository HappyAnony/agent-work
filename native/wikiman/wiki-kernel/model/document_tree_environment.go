// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"wiki-kernel/internal/doctree"
)

type documentTreeEnvironment struct {
	ctx      context.Context
	services doctree.Services
}

func listDocumentTreeWithEnvironment(q doctree.Request, e *documentTreeEnvironment) ([]*File, int, error) {
	if e == nil {
		return nil, 0, errors.New("document_tree_dependencies_unbound")
	}
	return doctree.List(e.ctx, q, e.services)
}
func (tx *Transaction) listDocumentTree(q doctree.Request) ([]*File, int, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.documentTree == nil {
			return nil, 0, errors.New("document_tree_dependencies_unbound")
		}
		e := *tx.environment.documentTree
		e.ctx = tx.environment.ctx
		e.services.Legacy = false
		return listDocumentTreeWithEnvironment(q, &e)
	}
	ctx := context.Background()
	if tx != nil && tx.environment != nil {
		ctx = tx.environment.ctx
	}
	return ListDocTreeContext(ctx, q.Box, q.Path, q.Sort, q.Flashcard, q.ShowHidden, q.Max)
}
