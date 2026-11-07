// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model/block.go folded-parent and fold-state rules.
package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
)

type FoldRequest struct{ Mode, ID string }
type FoldResult struct {
	ParentID         string
	IsFolded, IsRoot bool
}
type FoldServices struct {
	Load func(context.Context, string) (*parse.Tree, error)
}

var ErrFoldDependency = errors.New("block_fold_dependencies_unbound")

// Same predecessor-level/physical-parent rule as treenode.HeadingParent,
// with cancellation during potentially long sibling scans.
func foldHeadingParent(ctx context.Context, node *ast.Node) (*ast.Node, error) {
	level := 16
	if node.Type == ast.NodeHeading {
		level = node.HeadingLevel
	}
	for n := node.Previous; n != nil; n = n.Previous {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if n.Type == ast.NodeHeading && n.HeadingLevel < level {
			return n, nil
		}
	}
	return node.Parent, ctx.Err()
}

func Fold(ctx context.Context, q FoldRequest, s FoldServices) (ret FoldResult, err error) {
	if ctx == nil || s.Load == nil {
		return ret, ErrFoldDependency
	}
	if err = ctx.Err(); err != nil {
		return FoldResult{}, err
	}
	if q.Mode != "unfolded-parent" && q.Mode != "state" {
		return ret, errors.New("block_fold_mode")
	}
	tree, err := s.Load(ctx, q.ID)
	if err != nil {
		return FoldResult{}, err
	}
	if err = ctx.Err(); err != nil {
		return FoldResult{}, err
	}
	if tree == nil {
		return ret, nil
	}
	if tree.Root == nil {
		return ret, errors.New("block_fold_invalid_tree")
	}
	if q.Mode == "unfolded-parent" {
		node, e := relationNode(ctx, tree, q.ID)
		if e != nil {
			return FoldResult{}, e
		}
		if node == nil || !node.IsBlock() {
			return ret, nil
		}
		p, e := foldHeadingParent(ctx, node)
		if e != nil {
			return FoldResult{}, e
		}
		var folded *ast.Node
		for p != nil && p.Type != ast.NodeDocument {
			if e = ctx.Err(); e != nil {
				return FoldResult{}, e
			}
			if p.IALAttr("fold") != "1" {
				ret.ParentID = q.ID
				if folded != nil {
					ret.ParentID = folded.ID
				}
				if e = ctx.Err(); e != nil {
					return FoldResult{}, e
				}
				return ret, nil
			}
			folded = p
			ret.ParentID = p.ID
			p, e = foldHeadingParent(ctx, p)
			if e != nil {
				return FoldResult{}, e
			}
		}
		if ret.ParentID == "" {
			ret.ParentID = q.ID
		}
	} else {
		ret.IsRoot = tree.Root.ID == q.ID
		id := q.ID
		for i := 0; i < 32; i++ {
			if err = ctx.Err(); err != nil {
				return FoldResult{}, err
			}
			// getBlock stops on an empty ID before loading a tree. In
			// particular, never interpret an anonymous node as the root's parent.
			if id == "" {
				break
			}
			// getBlock rebuilt each ancestor from its source, not from main SQL.
			// Read only its fold and heading-parent fields: no render/OCR effects.
			t, e := s.Load(ctx, id)
			if e != nil {
				return FoldResult{}, e
			}
			if e = ctx.Err(); e != nil {
				return FoldResult{}, e
			}
			if t == nil {
				break
			}
			n, e := relationNode(ctx, t, id)
			if e != nil {
				return FoldResult{}, e
			}
			if n == nil {
				break
			}
			if n.IALAttr("fold") == "1" {
				ret.IsFolded = true
				break
			}
			p, e := foldHeadingParent(ctx, n)
			if e != nil {
				return FoldResult{}, e
			}
			id = ""
			if n.Type != ast.NodeDocument && p != nil {
				id = p.ID
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return FoldResult{}, err
	}
	return ret, nil
}
