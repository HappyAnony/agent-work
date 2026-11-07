package anchor

import (
	"context"
	"github.com/88250/lute/ast"
	"wiki-kernel/internal/indexdb"
)

func HeadingChildren(heading *ast.Node) (ret []*ast.Node) {
	start := heading.Next
	if start == nil {
		return
	}
	if start.Type == ast.NodeKramdownBlockIAL {
		start = start.Next
	}
	for n := start; n != nil; n = n.Next {
		if n.Type == ast.NodeHeading && heading.HeadingLevel >= n.HeadingLevel {
			break
		}
		ret = append(ret, n)
	}
	return
}

// The original parent-first-leaf/container-descendant/folded-heading reference
// expansion, including its duplicate rows. Callers supply their own cache/query.
func References(ctx context.Context, node *ast.Node, query func(context.Context, string) ([]indexdb.RefRecord, error)) (ret []indexdb.RefRecord, changed []*ast.Node, err error) {
	if node == nil || query == nil {
		return nil, nil, ErrDependency
	}
	nodes := map[string]*ast.Node{}
	ret, err = query(ctx, node.ID)
	if err != nil {
		return nil, nil, err
	}
	if node.Parent != nil && node.Parent.Type != ast.NodeDocument && node.Parent.IsContainerBlock() && node == FirstLeaf(node.Parent) {
		for p := node.Parent; p != nil && p.Type != ast.NodeDocument; p = p.Parent {
			refs, e := query(ctx, p.ID)
			if e != nil {
				return nil, nil, e
			}
			if len(refs) > 0 {
				ret = append(ret, refs...)
				nodes[p.ID] = p
			}
		}
	}
	if node.Type != ast.NodeDocument && node.IsContainerBlock() {
		ast.Walk(node, func(n *ast.Node, enter bool) ast.WalkStatus {
			if !enter || !n.IsBlock() {
				return ast.WalkContinue
			}
			var refs []indexdb.RefRecord
			refs, err = query(ctx, n.ID)
			if err != nil {
				return ast.WalkStop
			}
			if len(refs) > 0 {
				ret = append(ret, refs...)
				nodes[n.ID] = n
			}
			return ast.WalkContinue
		})
		if err != nil {
			return nil, nil, err
		}
	}
	if node.Type == ast.NodeHeading && node.IALAttr("fold") == "1" {
		for _, c := range HeadingChildren(node) {
			refs, e := query(ctx, c.ID)
			if e != nil {
				return nil, nil, e
			}
			if len(refs) > 0 {
				ret = append(ret, refs...)
				nodes[c.ID] = c
			}
		}
	}
	for _, n := range nodes {
		changed = append(changed, n)
	}
	return ret, changed, ctx.Err()
}
