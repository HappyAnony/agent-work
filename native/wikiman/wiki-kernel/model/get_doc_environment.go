// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"

	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"

	"wiki-kernel/internal/docview"
)

type getDocRequest = docview.Request
type getDocResult = docview.Result
type recentGetDocSnapshot = docview.Recent

// Native callers must hold their owner admission and exclusive candidate-tree
// access for this call. These callbacks receive the outer request context; no
// default loader, SQL connection, renderer or writer is installed implicitly.
type getDocEnvironment struct {
	ctx                       context.Context
	legacy, caseSensitive     bool
	dynamicLoadBlocks         int
	blockMissing, treeMissing error
	wait                      func(context.Context) error
	load                      func(context.Context, string) (*parse.Tree, error)
	parser                    func(context.Context) (*lute.Lute, error)
	backlink                  func(context.Context, *ast.Node, map[string]string) ([]*ast.Node, bool, error)
	references                func(context.Context, string) (map[string]int, error)
	virtualKeywords           func(context.Context, *ast.Node) ([]string, error)
	keywords                  func(context.Context, string, map[string]bool, int, string) ([]string, error)
	avNames                   func(context.Context, string) (string, error)
	virtual                   func(context.Context, *ast.Node, *[]*ast.Node, []string, map[string]int, *lute.Lute) (bool, error)
	render                    func(context.Context, *parse.Tree, *lute.Lute) (string, error)
	recent                    func(context.Context, recentGetDocSnapshot) error
}

func getDocumentWithEnvironment(q getDocRequest, e *getDocEnvironment) (getDocResult, error) {
	if e == nil {
		return getDocResult{}, errors.New("get_doc_dependencies_unbound")
	}
	return docview.Read(q, &docview.Services{Ctx: e.ctx, Legacy: e.legacy, CaseSensitive: e.caseSensitive, DynamicLoadBlocks: e.dynamicLoadBlocks, BlockMissing: e.blockMissing, TreeMissing: e.treeMissing, Wait: e.wait, Load: e.load, Parser: e.parser, Backlink: e.backlink, References: e.references, VirtualKeywords: e.virtualKeywords, Keywords: e.keywords, AvNames: e.avNames, Virtual: e.virtual, Render: e.render, Recent: e.recent})
}
func (tx *Transaction) getDocument(q getDocRequest) (getDocResult, error) {
	if tx != nil && tx.environment != nil && !tx.environment.legacy {
		if tx.environment.getDoc == nil {
			return getDocResult{}, errors.New("get_doc_dependencies_unbound")
		}
		e := *tx.environment.getDoc
		e.ctx = tx.environment.ctx
		e.legacy = false
		return getDocumentWithEnvironment(q, &e)
	}
	if tx != nil && tx.environment != nil && tx.environment.ctx != nil && tx.environment.legacy {
		return DocumentViewContext(tx.environment.ctx, q)
	}
	return getDocResult{}, errors.New("get_doc_dependencies_unbound")
}
