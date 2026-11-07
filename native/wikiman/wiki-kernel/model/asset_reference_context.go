// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"sort"
	"wiki-kernel/av"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/internal/docquery"
	"wiki-kernel/internal/indexrepair"
)

func UnusedAssetsContext(ctx context.Context) ([]string, error) {
	return checkAssetsContext(ctx, false)
}
func MissingAssetsContext(ctx context.Context) ([]string, error) {
	return checkAssetsContext(ctx, true)
}
func checkAssetsContext(ctx context.Context, missing bool) ([]string, error) {
	return runWorkspaceRead(ctx, func(o *workspaceReadOwner, c context.Context) (out []string, result error) {
		io, closeIO, e := o.r.openRepairIOChecked(c, false)
		if e != nil {
			return nil, e
		}
		defer func() {
			result = joinRepairCloseError(result, closeIO())
			if result != nil {
				out = nil
			}
		}()
		notebooks, e := o.base.owner.business.Notebooks(c)
		if e != nil {
			return nil, e
		}
		all, selected := []string{}, []string{}
		for _, box := range notebooks {
			all = append(all, box.ID)
			if !missing || !box.Closed {
				selected = append(selected, box.ID)
			}
		}
		assets, e := assetflow.Inventory(c, io.Source, all)
		if e != nil {
			return nil, e
		}
		s := assetflow.ReferenceServices{Assets: assets,
			Walk: func(c context.Context, visit func(*ast.Node) error) error {
				return docquery.WalkDocumentTrees(c, selected, io.Source.ReadDir, io.Source.Read, o.parser, func(tree *parse.Tree) error { return visit(tree.Root) })
			},
			AV: func(id string) (*av.AttributeView, error) { return assetflow.ReadAV(c, io.Source, id) },
			Exists: func(c context.Context, name string) (bool, error) {
				e := io.Source.Check(name, false)
				if indexrepair.IsSourceNotExist(e) {
					return false, nil
				}
				return e == nil, e
			},
			AVData: func(c context.Context) ([][]byte, error) { return assetflow.ReadAVData(c, io.Source) },
		}
		if missing {
			out = []string{}
			for _, id := range selected {
				s.Walk = func(c context.Context, visit func(*ast.Node) error) error {
					return docquery.WalkDocumentTrees(c, []string{id}, io.Source.ReadDir, io.Source.Read, o.parser, func(tree *parse.Tree) error { return visit(tree.Root) })
				}
				v, e := assetflow.CheckReferences(c, true, s)
				if e != nil {
					return nil, e
				}
				out = append(out, v...)
			}
			sort.Strings(out)
			return out, c.Err()
		}
		return assetflow.CheckReferences(c, false, s)
	})
}
