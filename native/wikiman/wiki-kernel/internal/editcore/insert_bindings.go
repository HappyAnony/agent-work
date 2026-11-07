// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package editcore

import (
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	av "wiki-kernel/internal/avmodel"
)

func InsertAttributeViewBindings(n *ast.Node, tree *parse.Tree, e *AttributeViewBoundEnvironment) error {
	if n == nil || tree == nil {
		return errors.New("invalid_av_bound_insert")
	}
	if n.Type != ast.NodeAttributeView {
		return nil
	}
	if err := e.Validate("insert"); err != nil {
		return err
	}
	var a *av.AttributeView
	if err := e.Guard(func() (err error) { a, err = e.Read(n.AttributeViewID); return }); err != nil {
		if e.Legacy && !e.StrictDependencies && e.Ctx.Err() == nil {
			return nil
		}
		return err
	}
	if a == nil || a.ID != n.AttributeViewID {
		return errors.New("invalid_av_bound_source")
	}
	if err := ValidateAttributeViewFieldSource(a); err != nil {
		return err
	}
	trees, nodes, err := CollectAttributeViewBoundNodes(a, e)
	if err != nil {
		return err
	}
	binding := &AttributeViewBindingEnvironment{StrictDependencies: e.StrictDependencies, Ctx: e.Ctx, Legacy: e.Legacy, Names: e.Names, Attrs: e.Attrs}
	for _, node := range nodes {
		if err := BindBlockAv0WithEnvironment(a.ID, node, trees[node.ID], true, binding); err != nil {
			return err
		}
	}
	if view := a.GetView(a.ViewID); view != nil {
		n.AttributeViewType = string(view.LayoutType)
		attrs := parse.IAL2Map(n.KramdownIAL)
		if attrs[av.NodeAttrView] == "" {
			attrs[av.NodeAttrView] = view.ID
			if err := e.Guard(func() error { return e.Attrs(n, tree, attrs) }); err != nil {
				return err
			}
		}
	}
	// doInsert staged the document before rebinding. Refresh even when an explicit
	// view already existed, so a snapshotting sink sees the final layout type.
	if err := e.Guard(func() error { return e.Tree(tree) }); err != nil {
		return err
	}
	if e.Complete != nil {
		return e.Complete()
	}
	return nil
}
