// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docview"
)

type ReferenceSwapPorts struct {
	NewID   func() string
	Updated func(*ast.Node)
}

// SwapReferences prepares the original swap on detached source trees.
func SwapReferences(ctx context.Context, refTree, defTree *parse.Tree, refID, defID string, includeChildren bool, ports ReferenceSwapPorts) (bool, error) {
	if ctx == nil || refTree == nil || defTree == nil || refTree.Root == nil || defTree.Root == nil || ports.NewID == nil || ports.Updated == nil {
		return false, errors.New("reference_swap_unbound")
	}
	if e := ctx.Err(); e != nil {
		return false, e
	}
	refNode := docview.GetNodeInTree(refTree, refID)
	defNode := docview.GetNodeInTree(defTree, defID)
	if refNode == nil || defNode == nil || refNode.Parent == nil || defNode.Parent == nil {
		return false, errors.New("reference_swap_block_missing")
	}
	if refNode.Parent.Type == ast.NodeListItem {
		refNode = refNode.Parent
	}
	if refNode == defNode {
		return false, nil
	}
	// Moving an ancestor into its descendant would corrupt the source tree.
	for n := refNode.Parent; n != nil; n = n.Parent {
		if n == defNode {
			return false, errors.New("reference_swap_ancestor")
		}
	}
	for n := defNode.Parent; n != nil; n = n.Parent {
		if n == refNode {
			return false, errors.New("reference_swap_ancestor")
		}
	}
	var defNodeChildren []*ast.Node
	if ast.NodeListItem == defNode.Parent.Type {
		defNode = defNode.Parent
	} else if ast.NodeHeading == defNode.Type && includeChildren {
		defNodeChildren = docview.HeadingChildren(defNode)
	}
	if ast.NodeListItem == defNode.Type {
		for c := defNode.FirstChild; nil != c; c = c.Next {
			if ast.NodeList == c.Type {
				defNodeChildren = append(defNodeChildren, c)
			}
		}
	}

	if refNode == defNode {
		return false, nil
	}
	for n := refNode.Parent; n != nil; n = n.Parent {
		if n == defNode {
			return false, errors.New("reference_swap_ancestor")
		}
	}
	for n := defNode.Parent; n != nil; n = n.Parent {
		if n == refNode {
			return false, errors.New("reference_swap_ancestor")
		}
	}
	if len(defNodeChildren) > 0 {
		for _, child := range defNodeChildren {
			for n := refNode; n != nil; n = n.Parent {
				if n == child {
					return false, errors.New("reference_swap_heading_child")
				}
			}
		}
	}
	ports.Updated(defNode)
	ports.Updated(refNode)

	refListType := 0
	if refNode.Type == ast.NodeListItem {
		refListType = refNode.Parent.ListData.Typ
	}
	refPivot := &ast.Node{Type: ast.NodeParagraph}
	refNode.InsertBefore(refPivot)

	if ast.NodeListItem == defNode.Type {
		if ast.NodeListItem == refNode.Type {
			if !includeChildren {
				for _, c := range defNodeChildren {
					refNode.AppendChild(c)
				}
			}
			defNode.InsertAfter(refNode)
			refPivot.InsertAfter(defNode)
		} else {
			newID := ports.NewID()
			li := &ast.Node{ID: newID, Type: ast.NodeListItem, ListData: &ast.ListData{Typ: defNode.Parent.ListData.Typ}}
			li.SetIALAttr("id", newID)
			li.SetIALAttr("updated", newID[:14])
			li.AppendChild(refNode)
			defNode.InsertAfter(li)
			if !includeChildren {
				for _, c := range defNodeChildren {
					li.AppendChild(c)
				}
			}

			newID = ports.NewID()
			list := &ast.Node{ID: newID, Type: ast.NodeList, ListData: &ast.ListData{Typ: defNode.Parent.ListData.Typ}}
			list.SetIALAttr("id", newID)
			list.SetIALAttr("updated", newID[:14])
			list.AppendChild(defNode)
			refPivot.InsertAfter(list)
		}
	} else {
		if ast.NodeListItem == refNode.Type {
			newID := ports.NewID()
			list := &ast.Node{ID: newID, Type: ast.NodeList, ListData: &ast.ListData{Typ: refListType}}
			list.SetIALAttr("id", newID)
			list.SetIALAttr("updated", newID[:14])
			list.AppendChild(refNode)
			defNode.InsertAfter(list)

			newID = ports.NewID()
			li := &ast.Node{ID: newID, Type: ast.NodeListItem, ListData: &ast.ListData{Typ: refListType}}
			li.SetIALAttr("id", newID)
			li.SetIALAttr("updated", newID[:14])
			li.AppendChild(defNode)
			for i := len(defNodeChildren) - 1; -1 < i; i-- {
				defNode.InsertAfter(defNodeChildren[i])
			}
			refPivot.InsertAfter(li)
		} else {
			defNode.InsertAfter(refNode)
			refPivot.InsertAfter(defNode)
			for i := len(defNodeChildren) - 1; -1 < i; i-- {
				defNode.InsertAfter(defNodeChildren[i])
			}
		}
	}
	refPivot.Unlink()

	return true, ctx.Err()
}

// TransferReferences updates dynamic anchor text only, preserving static text.
func TransferReferences(ctx context.Context, tree *parse.Tree, refID, fromID, toID, text string) (bool, error) {
	if ctx == nil || tree == nil || tree.Root == nil {
		return false, errors.New("reference_transfer_unbound")
	}
	if e := ctx.Err(); e != nil {
		return false, e
	}
	node := docview.GetNodeInTree(tree, refID)
	if node == nil {
		return false, errors.New("reference_transfer_block_missing")
	}
	changed := false
	for _, mark := range node.ChildrenByType(ast.NodeTextMark) {
		if mark.IsTextMarkType("block-ref") && mark.TextMarkBlockRefID == fromID {
			mark.TextMarkBlockRefID = toID
			if mark.TextMarkBlockRefSubtype == "d" {
				mark.TextMarkTextContent = text
			}
			changed = true
		}
	}
	return changed, ctx.Err()
}
