// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
	"wiki-kernel/internal/docview"
)

type HeadingOperation struct {
	Action, ID, ParentID, PreviousID, DOM string
	IgnoreProcess                         bool
}
type HeadingOperationsRequest struct {
	Mode, ID string
	Level    int
}
type HeadingOperationsPlan struct {
	Do, Undo   []HeadingOperation
	UnfoldedID string
	Changed    bool
}
type HeadingOperationsServices struct {
	Parser *lute.Lute
	NewID  func() string
	Counts func(context.Context, []string) (map[string]int, error)
}

func AppendHeadingChildren(ctx context.Context, tree *parse.Tree, id, dom string, parser *lute.Lute) (bool, error) {
	if ctx == nil || tree == nil || tree.Root == nil || parser == nil {
		return false, ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return false, e
	}
	heading := docview.GetNodeInTree(tree, id)
	if heading == nil || heading.Type != ast.NodeHeading {
		return false, nil
	}
	sub := parser.BlockDOM2Tree(dom)
	if sub == nil || sub.Root == nil {
		return false, errors.New("heading_children_dom_invalid")
	}
	var nodes []*ast.Node
	for n := sub.Root.FirstChild; n != nil; n = n.Next {
		if e := ctx.Err(); e != nil {
			return false, e
		}
		nodes = append(nodes, n)
	}
	slices.Reverse(nodes)
	for _, n := range nodes {
		if e := ctx.Err(); e != nil {
			return false, e
		}
		heading.InsertAfter(n)
	}
	return true, ctx.Err()
}

func UnfoldHeading(ctx context.Context, heading *ast.Node) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if heading == nil {
		return nil
	}
	for _, child := range docview.HeadingChildren(heading) {
		ast.Walk(child, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if enter && n.IsBlock() {
				n.RemoveIALAttr("fold")
				n.RemoveIALAttr("heading-fold")
			}
			return ast.WalkContinue
		})
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	heading.RemoveIALAttr("fold")
	heading.RemoveIALAttr("heading-fold")
	return ctx.Err()
}

// PrepareHeadingOperations returns original UI transaction values only. It
// never saves the source, indexes candidates, or publishes editor events.
func PrepareHeadingOperations(ctx context.Context, tree *parse.Tree, q HeadingOperationsRequest, s HeadingOperationsServices) (out HeadingOperationsPlan, err error) {
	if ctx == nil || tree == nil || tree.Root == nil || s.Parser == nil || s.Parser.RenderOptions == nil {
		return out, ErrDependency
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	node := docview.GetNodeInTree(tree, q.ID)
	if node == nil {
		return out, ErrMissing
	}
	if node.Type != ast.NodeHeading {
		return out, nil
	}
	parser := *s.Parser
	options := *s.Parser.RenderOptions
	parser.RenderOptions = &options
	nodes := append([]*ast.Node{node}, docview.HeadingChildren(node)...)
	switch q.Mode {
	case "delete", "insert":
		if q.Mode == "insert" && s.NewID == nil {
			return out, ErrDependency
		}
		for _, n := range nodes {
			if err = ctx.Err(); err != nil {
				return HeadingOperationsPlan{}, err
			}
			if q.Mode == "delete" {
				undo := HeadingOperation{Action: "insert", ID: n.ID, DOM: parser.RenderNodeBlockDOM(n)}
				if n.Parent != nil {
					undo.ParentID = n.Parent.ID
				}
				if n.Previous != nil {
					undo.PreviousID = n.Previous.ID
				}
				out.Do = append(out.Do, HeadingOperation{Action: "delete", ID: n.ID})
				out.Undo = append(out.Undo, undo)
			} else {
				n.ID = s.NewID()
				if !ast.IsNodeIDPattern(n.ID) {
					return HeadingOperationsPlan{}, errors.New("heading_operation_id_invalid")
				}
				n.SetIALAttr("id", n.ID)
				out.Do = append(out.Do, HeadingOperation{Action: "insert", ID: n.ID, DOM: parser.RenderNodeBlockDOM(n), IgnoreProcess: true})
				out.Undo = append(out.Undo, HeadingOperation{Action: "delete", ID: n.ID})
			}
		}
	case "level":
		if q.Level < 1 || q.Level > 6 {
			return out, errors.New("heading_level_invalid")
		}
		if q.Level == node.HeadingLevel {
			return out, nil
		}
		if s.Counts == nil {
			return out, ErrDependency
		}
		diff := q.Level - node.HeadingLevel
		var headings []*ast.Node
		for _, n := range nodes {
			headings = append(headings, n.ChildrenByType(ast.NodeHeading)...)
		}
		if err = docview.FillReferenceCounts(ctx, headings, s.Counts); err != nil {
			return HeadingOperationsPlan{}, err
		}
		if node.IALAttr("fold") == "1" {
			if err = UnfoldHeading(ctx, node); err != nil {
				return HeadingOperationsPlan{}, err
			}
			out.UnfoldedID = node.ID
		}
		for _, n := range headings {
			if err = ctx.Err(); err != nil {
				return HeadingOperationsPlan{}, err
			}
			out.Undo = append(out.Undo, HeadingOperation{Action: "update", ID: n.ID, DOM: parser.RenderNodeBlockDOM(n)})
			n.HeadingLevel = max(1, min(6, n.HeadingLevel+diff))
			out.Do = append(out.Do, HeadingOperation{Action: "update", ID: n.ID, DOM: parser.RenderNodeBlockDOM(n)})
		}
	default:
		return out, errors.New("heading_operation_mode_invalid")
	}
	out.Changed = true
	return out, ctx.Err()
}
