// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/docview"
)

type HeadingMoveRequest struct{ ID, PreviousID, ParentID string }
type HeadingMoveError struct {
	ID       string
	Language int
}

func (e *HeadingMoveError) Error() string {
	return fmt.Sprintf("heading_move_invalid:%s:%d", e.ID, e.Language)
}

// MoveHeading prepares the original outline move on a detached tree. History
// runs before any mutation; publication belongs to the enclosing owner.
func MoveHeading(ctx context.Context, tree *parse.Tree, q HeadingMoveRequest, history func(context.Context, *parse.Tree) error) (bool, error) {
	if ctx == nil || tree == nil || tree.Root == nil || history == nil {
		return false, errors.New("heading_move_dependencies_unbound")
	}
	if e := ctx.Err(); e != nil {
		return false, e
	}
	headingID, previousID, parentID := q.ID, q.PreviousID, q.ParentID

	if headingID == parentID || headingID == previousID {
		return false, ctx.Err()
	}

	heading := docview.GetNodeInTree(tree, headingID)
	if nil == heading || heading.Type != ast.NodeHeading || heading.Parent == nil {
		return false, &HeadingMoveError{ID: headingID}
	}

	if ast.NodeDocument != heading.Parent.Type {
		// 仅支持文档根节点下第一层标题，不支持容器块内标题
		return false, &HeadingMoveError{ID: headingID, Language: 240}
	}

	headingChildren := docview.HeadingChildren(heading)

	if "" != previousID {
		previousHeading := docview.GetNodeInTree(tree, previousID)
		if nil == previousHeading || previousHeading.Type != ast.NodeHeading || previousHeading.Parent == nil {
			return false, &HeadingMoveError{ID: previousID}
		}

		if ast.NodeDocument != previousHeading.Parent.Type {
			// 仅支持文档根节点下第一层标题，不支持容器块内标题
			return false, &HeadingMoveError{ID: headingID, Language: 248}
		}

		for _, h := range headingChildren {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if h.ID == previousID {
				// 不能移动到自己的子标题下
				return false, &HeadingMoveError{ID: headingID, Language: 241}
			}
		}

		if err := history(ctx, tree); err != nil {
			return false, err
		}

		targetNode := previousHeading
		previousHeadingChildren := docview.HeadingChildren(previousHeading)
		if 0 < len(previousHeadingChildren) {
			targetNode = previousHeadingChildren[len(previousHeadingChildren)-1]
		}

		for _, h := range headingChildren {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if h.ID == targetNode.ID { // 目标节点是当前标题的子节点
				targetNode = heading.Previous
			}
		}
		if targetNode.ID == heading.ID {
			targetNode = heading.Previous
		}

		diffLevel := heading.HeadingLevel - previousHeading.HeadingLevel
		heading.HeadingLevel = previousHeading.HeadingLevel

		for i := len(headingChildren) - 1; i >= 0; i-- {
			child := headingChildren[i]
			if ast.NodeHeading == child.Type {
				child.HeadingLevel -= diffLevel
				if 6 < child.HeadingLevel {
					child.HeadingLevel = 6
				}
			}
			targetNode.InsertAfter(child)
		}
		targetNode.InsertAfter(heading)
	} else if "" != parentID {
		parentHeading := docview.GetNodeInTree(tree, parentID)
		if nil == parentHeading || parentHeading.Type != ast.NodeHeading || parentHeading.Parent == nil {
			return false, &HeadingMoveError{ID: parentID}
		}

		if ast.NodeDocument != parentHeading.Parent.Type {
			// 仅支持文档根节点下第一层标题，不支持容器块内标题
			return false, &HeadingMoveError{ID: headingID, Language: 248}
		}

		for _, h := range headingChildren {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if h.ID == parentID {
				// 不能移动到自己的子标题下
				return false, &HeadingMoveError{ID: headingID, Language: 241}
			}
		}

		if err := history(ctx, tree); err != nil {
			return false, err
		}

		targetNode := parentHeading
		parentHeadingChildren := docview.HeadingChildren(parentHeading)
		// 找到下方第一个非标题节点
		var tmp []*ast.Node
		for _, child := range parentHeadingChildren {
			if ast.NodeHeading == child.Type {
				break
			}
			tmp = append(tmp, child)
		}
		parentHeadingChildren = tmp
		if 0 < len(parentHeadingChildren) {
			for _, child := range parentHeadingChildren {
				if child.ID == headingID {
					break
				}
				targetNode = child
			}
		}

		diffLevel := heading.HeadingLevel - parentHeading.HeadingLevel - 1
		heading.HeadingLevel = parentHeading.HeadingLevel + 1
		if 6 < heading.HeadingLevel {
			heading.HeadingLevel = 6
		}

		for i := len(headingChildren) - 1; i >= 0; i-- {
			child := headingChildren[i]
			if ast.NodeHeading == child.Type {
				child.HeadingLevel -= diffLevel
				if 6 < child.HeadingLevel {
					child.HeadingLevel = 6
				}
			}
			targetNode.InsertAfter(child)
		}
		targetNode.InsertAfter(heading)
	} else {
		if err := history(ctx, tree); err != nil {
			return false, err
		}

		// 移到第一个标题前
		var firstHeading *ast.Node
		for n := tree.Root.FirstChild; nil != n; n = n.Next {
			if ast.NodeHeading == n.Type {
				firstHeading = n
				break
			}
		}
		if nil == firstHeading || firstHeading.ID == heading.ID {
			return false, ctx.Err()
		}

		diffLevel := heading.HeadingLevel - firstHeading.HeadingLevel
		heading.HeadingLevel = firstHeading.HeadingLevel

		firstHeading.InsertBefore(heading)
		for i := 0; i < len(headingChildren); i++ {
			child := headingChildren[i]
			if ast.NodeHeading == child.Type {
				child.HeadingLevel -= diffLevel
				if 6 < child.HeadingLevel {
					child.HeadingLevel = 6
				}
			}
			firstHeading.InsertBefore(child)
		}
	}

	return true, ctx.Err()
}
