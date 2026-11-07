// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/treenode"
)

type exportMergeServices struct {
	guard func() error
	list  func(string, string) ([]*File, error)
	load  func(string, string) (*parse.Tree, error)
}

func exportMergeGuard(ports []*exportMergeServices) error {
	if len(ports) == 0 {
		return nil
	}
	p := ports[0]
	if p == nil || p.guard == nil || p.list == nil || p.load == nil {
		return errors.New("export_merge_dependencies_unbound")
	}
	return p.guard()
}

// File controllers can admit all document preparation before taking output or
// device-resource capabilities. No legacy IO fallback is exposed by this port.
func (m *markdownExportContext) prepareHTMLDocument(id string, merge bool) (tree *parse.Tree, node *ast.Node, bt *treenode.BlockTree, err error) {
	if err = m.guard(); err != nil {
		return
	}
	tree, bt, err = m.prepareFocusedExport(id)
	if err != nil {
		return nil, nil, nil, err
	}
	node = treenode.GetNodeInTree(tree, id)
	if !merge {
		return
	}
	if !m.mergeBound {
		return nil, nil, nil, errors.New("export_merge_preferences_unbound")
	}
	services, closeServices, err := m.mergeServices(tree.Box)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		err = errors.Join(err, closeServices())
		if err != nil {
			tree, node, bt = nil, nil, nil
		}
	}()
	tree, err = mergeSubDocs(tree, services)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = m.checkExportRenderTree(tree); err != nil {
		return nil, nil, nil, err
	}
	return
}
