// SPDX-License-Identifier: AGPL-3.0-or-later
package blockview

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"slices"
	"strconv"
)

func OutlineView(ctx context.Context, tree *parse.Tree, parser *lute.Lute, addTitle bool, storage map[string]interface{}) (out []*Path, err error) {
	if ctx == nil || tree == nil || tree.Root == nil || parser == nil {
		return nil, errors.New("outline_dependencies_unbound")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	out = []*Path{}
	var expand []string
	explicit := storage["expandIds"] != nil
	if explicit {
		values, ok := storage["expandIds"].([]interface{})
		if !ok {
			return nil, errors.New("outline_expand_ids_invalid")
		}
		for _, value := range values {
			id, ok := value.(string)
			if !ok {
				return nil, errors.New("outline_expand_id_invalid")
			}
			expand = append(expand, id)
		}
	}
	if addTitle {
		title := &ast.Node{ID: tree.Root.ID, Type: ast.NodeHeading, HeadingLevel: 1}
		title.KramdownIAL = make([][]string, len(tree.Root.KramdownIAL))
		for i, v := range tree.Root.KramdownIAL {
			title.KramdownIAL[i] = slices.Clone(v)
		}
		title.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(tree.Root.IALAttr("title"))})
		tree.Root.PrependChild(title)
	}
	var roots, stack []*Block
	ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil {
			return ast.WalkStop
		}
		if !enter || n.Type != ast.NodeHeading || n.ParentIs(ast.NodeBlockquote) {
			return ast.WalkContinue
		}
		n.Box, n.Path = tree.Box, tree.Path
		subType := ""
		if n.HeadingLevel >= 1 && n.HeadingLevel <= 6 {
			subType = "h" + strconv.Itoa(n.HeadingLevel)
		}
		b := &Block{RootID: tree.Root.ID, Depth: n.HeadingLevel, Box: tree.Box, Path: tree.Path, ID: n.ID, Content: Outline(n, parser), Type: n.Type.String(), SubType: subType, Folded: true}
		for len(stack) > 0 && stack[len(stack)-1].Depth >= b.Depth {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, b)
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, b)
		}
		stack = append(stack, b)
		return ast.WalkSkipChildren
	})
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var visit func(*Block, int) error
	visit = func(b *Block, depth int) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		b.Depth = depth
		b.Count = len(b.Children)
		if len(storage) == 0 {
			b.Folded = false
		}
		if explicit {
			b.Folded = !slices.Contains(expand, b.ID)
		}
		for _, child := range b.Children {
			if e := visit(child, depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	for _, b := range roots {
		if err = visit(b, 0); err != nil {
			return nil, err
		}
		out = append(out, &Path{ID: b.ID, Box: b.Box, Name: b.Content, NodeType: b.Type, Type: "outline", SubType: b.SubType, Blocks: b.Children, Depth: 0, Count: b.Count, Folded: b.Folded})
	}
	return out, ctx.Err()
}
