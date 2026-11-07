// SPDX-License-Identifier: AGPL-3.0-or-later
// Original model/block.go metadata/sibling/relevant-ID algorithms.
package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
)

type BlockTreeInfo struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	ParentID     string `json:"parentID"`
	ParentType   string `json:"parentType"`
	PreviousID   string `json:"previousID"`
	PreviousType string `json:"previousType"`
	NextID       string `json:"nextID"`
	NextType     string `json:"nextType"`
}
type RelationIDs struct{ Parent, Previous, Next string }
type RelationRequest struct {
	Mode, ID string
	IDs      []string
}
type RelationResult struct {
	Infos map[string]*BlockTreeInfo
	IDs   RelationIDs
}
type RelationServices struct {
	Trees        func(context.Context, []string) (map[string]*parse.Tree, error)
	Load         func(context.Context, string) (*parse.Tree, error)
	BlockMissing error
}

var ErrRelationDependency = errors.New("block_relation_dependencies_unbound")

func relationNode(ctx context.Context, t *parse.Tree, id string) (node *ast.Node, err error) {
	if t == nil || t.Root == nil {
		return nil, errors.New("block_relation_invalid_tree")
	}
	ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if enter && n.ID == id {
			node = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return node, ctx.Err()
}
func relationParent(ctx context.Context, n *ast.Node) (*ast.Node, error) {
	for p := n.Parent; p != nil; p = p.Parent {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if p.ID != "" && p.IsBlock() {
			return p, nil
		}
	}
	return nil, ctx.Err()
}
func relationNeighbor(ctx context.Context, n *ast.Node, previous bool) (*ast.Node, error) {
	for {
		if previous {
			n = n.Previous
		} else {
			n = n.Next
		}
		if n == nil {
			return nil, ctx.Err()
		}
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if n.ID != "" && n.IsBlock() {
			return n, nil
		}
	}
}
func relationFirstBlock(ctx context.Context, n *ast.Node) (ret *ast.Node, err error) {
	// The original helper includes n itself. Do not replace it with first leaf.
	ast.Walk(n, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if enter && n.IsBlock() {
			ret = n
			return ast.WalkStop
		}
		return ast.WalkContinue
	})
	return ret, ctx.Err()
}

func Relations(ctx context.Context, q RelationRequest, s RelationServices) (ret RelationResult, err error) {
	if ctx == nil {
		return ret, ErrRelationDependency
	}
	if err = ctx.Err(); err != nil {
		return RelationResult{}, err
	}
	q.IDs = slices.Clone(q.IDs)
	if q.Mode == "infos" {
		if s.Trees == nil {
			return ret, ErrRelationDependency
		}
		ret.Infos = map[string]*BlockTreeInfo{}
		if len(q.IDs) == 0 {
			return ret, nil
		}
		trees, e := s.Trees(ctx, slices.Clone(q.IDs))
		if e != nil {
			return RelationResult{}, e
		}
		if e = ctx.Err(); e != nil {
			return RelationResult{}, e
		}
		for id, t := range trees {
			n, e := relationNode(ctx, t, id)
			if e != nil {
				return RelationResult{}, e
			}
			info := &BlockTreeInfo{ID: id}
			ret.Infos[id] = info
			if n == nil {
				continue
			}
			info.Type = n.Type.String()
			p, e := relationParent(ctx, n)
			if e != nil {
				return RelationResult{}, e
			}
			if p != nil {
				info.ParentID, info.ParentType = p.ID, p.Type.String()
			}
			prev, e := relationNeighbor(ctx, n, true)
			if e != nil {
				return RelationResult{}, e
			}
			if prev != nil {
				info.PreviousID, info.PreviousType = prev.ID, prev.Type.String()
			}
			next, e := relationNeighbor(ctx, n, false)
			if e != nil {
				return RelationResult{}, e
			}
			if next != nil {
				info.NextID, info.NextType = next.ID, next.Type.String()
			}
		}
	} else {
		if q.Mode != "sibling" && q.Mode != "relevant" {
			return ret, errors.New("block_relation_mode")
		}
		if s.Load == nil || (q.Mode == "relevant" && s.BlockMissing == nil) {
			return ret, ErrRelationDependency
		}
		t, e := s.Load(ctx, q.ID)
		if e != nil {
			return RelationResult{}, e
		}
		if e = ctx.Err(); e != nil {
			return RelationResult{}, e
		}
		if t == nil {
			if q.Mode == "relevant" {
				return RelationResult{}, s.BlockMissing
			}
			return ret, nil
		}
		n, e := relationNode(ctx, t, q.ID)
		if e != nil {
			return RelationResult{}, e
		}
		if n == nil {
			if q.Mode == "relevant" {
				return RelationResult{}, s.BlockMissing
			}
			return ret, nil
		}
		if q.Mode == "relevant" {
			if n.Parent != nil {
				ret.IDs.Parent = n.Parent.ID
			}
			prev, next := n.Previous, n.Next
			if prev != nil && prev.Type == ast.NodeKramdownBlockIAL {
				prev = prev.Previous
			}
			if next != nil && next.Type == ast.NodeKramdownBlockIAL {
				next = next.Next
			}
			if prev != nil {
				ret.IDs.Previous = prev.ID
			}
			if next != nil {
				ret.IDs.Next = next.ID
			}
		} else {
			if !n.IsBlock() {
				return ret, nil
			}
			p, e := relationParent(ctx, n)
			if e != nil {
				return RelationResult{}, e
			}
			if p == nil {
				return ret, nil
			}
			ret.IDs.Parent = p.ID
			neighbor := func(n *ast.Node) (string, error) {
				if n == nil || !n.IsBlock() {
					return "", ctx.Err()
				}
				first, e := relationFirstBlock(ctx, n)
				if e != nil {
					return "", e
				}
				if first != nil {
					return first.ID, nil
				}
				return n.ID, nil
			}
			if p.Type == ast.NodeDocument {
				ret.IDs.Previous, e = neighbor(n.Previous)
				if e != nil {
					return RelationResult{}, e
				}
				ret.IDs.Next, e = neighbor(n.Next)
				if e != nil {
					return RelationResult{}, e
				}
			} else {
				for parent := p; parent != nil; {
					if e = ctx.Err(); e != nil {
						return RelationResult{}, e
					}
					if parent.Previous != nil && parent.Previous.IsBlock() {
						ret.IDs.Previous, e = neighbor(parent.Previous)
						if e != nil {
							return RelationResult{}, e
						}
						break
					}
					parent, e = relationParent(ctx, parent)
					if e != nil {
						return RelationResult{}, e
					}
				}
				for parent := p; parent != nil; {
					if e = ctx.Err(); e != nil {
						return RelationResult{}, e
					}
					if parent.Next != nil && parent.Next.IsBlock() {
						ret.IDs.Next, e = neighbor(parent.Next)
						if e != nil {
							return RelationResult{}, e
						}
						break
					}
					parent, e = relationParent(ctx, parent)
					if e != nil {
						return RelationResult{}, e
					}
				}
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return RelationResult{}, err
	}
	return ret, nil
}
