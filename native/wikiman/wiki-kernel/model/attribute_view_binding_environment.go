// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/editcore"
	// A projection of existing cell/bound owner services, not another source writer.
	// attrs owns the original normalization, tree/cache and notification effects.
)

type attributeViewBindingEnvironment struct {
	strictDependencies bool
	ctx                context.Context
	legacy             bool
	node               func(string) (*ast.Node, *parse.Tree, error)
	names              func(string) (string, error)
	attrs              func(*ast.Node, *parse.Tree, map[string]string) error
}

func cellAttributeViewBindingEnvironment(e *attributeViewCellEnvironment) *attributeViewBindingEnvironment {
	return r04FromAttributeViewBindingEnvironment(editcore.CellAttributeViewBindingEnvironment(e.r04Core()))
}

func (e *attributeViewBindingEnvironment) validate(load bool) error {
	return e.r04Core().Validate(load)
}

func (e *attributeViewBindingEnvironment) guard(fn func() error) error { return e.r04Core().Guard(fn) }

func bindBlockAvWithEnvironment(avID, blockID string, bind bool, e *attributeViewBindingEnvironment) error {
	return editcore.BindBlockAvWithEnvironment(avID, blockID, bind, e.r04Core())
}

func bindBlockAv0WithEnvironment(avID string, node *ast.Node, tree *parse.Tree, bind bool, e *attributeViewBindingEnvironment) error {
	return editcore.BindBlockAv0WithEnvironment(avID, node, tree, bind, e.r04Core())
}
