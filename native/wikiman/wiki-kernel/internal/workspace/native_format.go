// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package workspace

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"time"
	"wiki-kernel/internal/doclife"
	"wiki-kernel/internal/indexdb"
)

type NativeFormatPlan struct {
	Tree         *parse.Tree
	SourceHashes map[string][32]byte
}

func (l *Lease) PlanNativeFormat(ctx context.Context, id string) (ret NativeFormatPlan, err error) {
	err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
		base, hashes, guard, e := l.workspace.resources.documentLifecycleServices(ctx, l.workspace, b, "")
		if e != nil {
			return e
		}
		loc, ok, e := base.Lookup(ctx, id)
		if e != nil {
			return e
		}
		if !ok {
			return ErrDocument
		}
		tree, e := base.Load(ctx, loc.Box, loc.Path)
		if e != nil {
			return e
		}
		parser, e := base.Parser(ctx)
		if e != nil {
			return e
		}
		ret.Tree, e = doclife.Format(ctx, tree, parser, doclife.TemplateBodyServices{NewID: ast.NewNodeID, Now: time.Now})
		if e != nil {
			return e
		}
		ret.SourceHashes = hashes
		return guard.Confirm(ctx)
	})
	if err != nil {
		return NativeFormatPlan{}, err
	}
	return
}
