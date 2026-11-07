// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"wiki-kernel/av"
	"wiki-kernel/treenode"
)

// Both adapters retain the original traversal, reference order, per-root cycle
// guard and detached-row exclusion. Missing index entries/AV files are dangling
// references; failures reading an existing dependency are errors in the owned port.
type exportReferenceServices struct {
	guard         func() error
	resolve       func(string) (*parse.Tree, error)
	attributeView func(string) (*av.AttributeView, error)
}

func walkExportReferences(tree *parse.Tree, ids *[]string, trees map[string]*parse.Tree, services exportReferenceServices) error {
	if err := services.guard(); err != nil {
		return err
	}
	if tree == nil || tree.Root == nil || ids == nil || trees == nil {
		return errors.New("invalid_export_reference_tree")
	}
	if trees[tree.ID] != nil {
		return nil
	}
	trees[tree.ID] = tree
	var failure error
	follow := func(id, recordedID string) (bool, error) {
		candidate, err := services.resolve(id)
		if err != nil {
			return false, err
		}
		if candidate == nil {
			return false, nil
		}
		*ids = append(*ids, recordedID)
		return true, walkExportReferences(candidate, ids, trees, services)
	}
	ast.Walk(tree.Root, func(n *ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.WalkContinue
		}
		if failure = services.guard(); failure != nil {
			return ast.WalkStop
		}
		if treenode.IsBlockRef(n) || treenode.IsBlockLink(n) {
			var id string
			if treenode.IsBlockRef(n) {
				id, _, _ = treenode.GetBlockRef(n)
			} else {
				id = strings.TrimPrefix(n.TextMarkAHref, "siyuan://blocks/")
			}
			if id == "" {
				return ast.WalkContinue
			}
			found, err := follow(id, id)
			if failure = err; failure != nil {
				return ast.WalkStop
			}
			if !found {
				return ast.WalkSkipChildren
			}
			return ast.WalkContinue
		}
		if n.Type != ast.NodeAttributeView || n.AttributeViewID == "" {
			return ast.WalkContinue
		}
		var view *av.AttributeView
		view, failure = services.attributeView(n.AttributeViewID)
		if failure != nil {
			return ast.WalkStop
		}
		if view == nil {
			return ast.WalkContinue
		}
		values := view.GetBlockKeyValues()
		if values == nil {
			return ast.WalkContinue
		}
		for _, value := range values.Values {
			if value == nil {
				failure = errors.New("invalid_export_reference_value")
				return ast.WalkStop
			}
			if value.IsDetached {
				continue
			}
			if value.Block == nil {
				failure = errors.New("invalid_export_reference_block")
				return ast.WalkStop
			}
			// Keep the old distinction between lookup Block.ID and recorded BlockID.
			if _, failure = follow(value.Block.ID, value.BlockID); failure != nil {
				return ast.WalkStop
			}
		}
		return ast.WalkContinue
	})
	if failure != nil {
		return failure
	}
	*ids = gulu.Str.RemoveDuplicatedElem(*ids)
	return services.guard()
}

func (e *archiveExportContext) references(tree *parse.Tree, ids *[]string, trees, cache map[string]*parse.Tree) error {
	return walkExportReferences(tree, ids, trees, exportReferenceServices{
		guard: e.guard,
		resolve: func(id string) (*parse.Tree, error) {
			if err := e.guard(); err != nil {
				return nil, err
			}
			block, err := e.binding.scope.commit.r.referenceLookup(e.binding.scope.ctx, id)
			if err != nil || block == nil {
				return nil, err
			}
			if _, err = e.box(block.BoxID); err != nil {
				return nil, err
			}
			if candidate := cache[block.RootID]; candidate != nil {
				if candidate.ID != block.RootID || candidate.Box != block.BoxID || candidate.Path != block.Path {
					return nil, errors.New("export_reference_identity_changed")
				}
				return candidate, e.guard()
			}
			loaded, err := e.selectedTrees(block.BoxID, []string{block.Path})
			if err != nil {
				return nil, err
			}
			candidate := loaded[block.RootID]
			if candidate == nil {
				return nil, errors.New("export_reference_root_mismatch")
			}
			cache[block.RootID] = candidate
			return candidate, e.guard()
		},
		attributeView: e.readExportView,
	})
}
