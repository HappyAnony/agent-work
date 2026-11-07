// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/treenode"
)

type exportFocusBinding struct {
	root     *ast.Node
	location treenode.BlockTree
}

// Only a candidate created here may bypass the saved-tree pointer identity
// check. Caller-provided clones never gain read/write authority by sharing IDs.
func (m *markdownExportContext) checkExportRenderTree(tree *parse.Tree) error {
	if err := m.guard(); err != nil {
		return err
	}
	if tree == nil {
		return errors.New("export_render_tree_unbound")
	}
	saved, ok := m.read.documents[tree.ID]
	if !ok {
		return errors.New("export_render_tree_unbound")
	}
	if tree == saved.tree {
		return saved.check(tree, tree.Box, tree.Path)
	}
	binding, ok := m.focusTrees[tree]
	if !ok {
		return errors.New("export_focus_tree_unbound")
	}
	b := binding.location
	if err := saved.check(saved.tree, b.BoxID, b.Path); err != nil {
		return err
	}
	if tree.Root != binding.root || tree.Root == nil || tree.Root.Type != ast.NodeDocument || tree.ID != b.RootID || tree.Root.ID != b.RootID || tree.Box != b.BoxID || tree.Path != b.Path || tree.HPath != b.HPath || tree.Root.Box != b.BoxID || tree.Root.Path != b.Path {
		return errors.New("export_focus_identity_changed")
	}
	return m.guard()
}
func (m *markdownExportContext) prepareFocusedExport(id string) (tree *parse.Tree, location *treenode.BlockTree, err error) {
	p := m.referenceRenderer()
	bt := exportRenderLookup(id, []*exportReferenceRenderer{p})
	if err = p.check(); err != nil {
		return nil, nil, err
	}
	if bt == nil {
		return nil, nil, ErrBlockNotFound
	}
	source, err := m.embeddedTree(id)
	if err != nil {
		return nil, nil, err
	}
	node := treenode.GetNodeInTree(source, id)
	if node == nil {
		return nil, nil, ErrBlockNotFound
	}
	if bt.Type == "d" && node.Type != ast.NodeDocument || bt.Type == "h" && node.Type != ast.NodeHeading {
		return nil, nil, errors.New("export_focus_type_changed")
	}
	tree = selectExportTree(bt, source, p.parser())
	// The original focused selector creates a temporary root. Contextual readers
	// use the stable source root ID for both Tree and Root; source bytes are intact.
	tree.Root.ID = bt.RootID
	tree.Root.Box, tree.Root.Path = bt.BoxID, bt.Path
	if m.focusTrees == nil {
		m.focusTrees = map[*parse.Tree]exportFocusBinding{}
	}
	m.focusTrees[tree] = exportFocusBinding{root: tree.Root, location: *bt}
	if err = m.checkExportRenderTree(tree); err != nil {
		delete(m.focusTrees, tree)
		return nil, nil, err
	}
	result := *bt
	return tree, &result, nil
}
func adjustExportHeading(bt *treenode.BlockTree, tree *parse.Tree, addTitle bool, ports []*exportReferenceRenderer) {
	if len(ports) == 0 {
		adjustHeadingLevel(bt, tree)
		return
	}
	if !exportRenderReady(ports) {
		return
	}
	if bt == nil || tree == nil || tree.Root == nil {
		exportRenderFailure(ports, errors.New("export_heading_unbound"))
		return
	}
	// The legacy helper assumes a title precedes the focused headings when enabled.
	// An empty document must still be a safe no-op instead of dereferencing nil.
	if tree.Root.FirstChild == nil {
		return
	}
	adjustHeadingLevelWithTitle(bt, tree, addTitle)
	exportRenderReady(ports)
}
