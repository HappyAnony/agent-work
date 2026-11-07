// SPDX-License-Identifier: AGPL-3.0-or-later
// Original blockinfo.go positioning and breadcrumb rules, with owned services.
package docquery

import (
	"context"
	"errors"
	"github.com/88250/gulu"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"slices"
	"strconv"
	"strings"
	"wiki-kernel/internal/anchor"
	"wiki-kernel/internal/blockview"
)

type BlockPath = blockview.BlockPath
type PositionRequest struct {
	Mode, ID, Content string
	IDs, ExcludeTypes []string
	Embed             bool
	HeadingMode       int
}
type PositionResult struct {
	Index      int
	Indexes    map[string]int
	Breadcrumb []*BlockPath
	Text       string
}
type PositionServices struct {
	Load     func(context.Context, string) (*parse.Tree, error)
	Notebook func(context.Context, string) (string, bool, error)
	RootPath func(context.Context, string, string) (string, error)
	AVName   func(context.Context, string) (string, error)
	Text     func(context.Context, *ast.Node, []string, bool) (string, error)
	Parser   func(context.Context) (*lute.Lute, error)
}

var ErrPositionDependency = errors.New("block_position_dependencies_unbound")

func Position(ctx context.Context, q PositionRequest, s PositionServices) (ret PositionResult, err error) {
	if ctx == nil {
		return ret, ErrPositionDependency
	}
	if err = ctx.Err(); err != nil {
		return PositionResult{}, err
	}
	q.IDs = slices.Clone(q.IDs)
	q.ExcludeTypes = slices.Clone(q.ExcludeTypes)
	if q.Mode == "dom-text" {
		if s.Parser == nil || s.Text == nil {
			return PositionResult{}, ErrPositionDependency
		}
		p, e := s.Parser(ctx)
		if e != nil {
			return PositionResult{}, e
		}
		if e = ctx.Err(); e != nil {
			return PositionResult{}, e
		}
		if p == nil || p.ParseOptions == nil || p.RenderOptions == nil {
			return PositionResult{}, ErrPositionDependency
		}
		t := p.BlockDOM2Tree(q.Content)
		if t == nil || t.Root == nil {
			return PositionResult{}, errors.New("block_position_invalid_tree")
		}
		ret.Text, e = s.Text(ctx, t.Root.FirstChild, nil, true)
		if e != nil {
			return PositionResult{}, e
		}
		if e = ctx.Err(); e != nil {
			return PositionResult{}, e
		}
		return ret, nil
	}
	if s.Load == nil {
		return ret, ErrPositionDependency
	}
	id := q.ID
	switch q.Mode {
	case "index":
	case "indexes":
		ret.Indexes = map[string]int{}
		if len(q.IDs) == 0 {
			return ret, nil
		}
		id = q.IDs[0]
	case "breadcrumb":
		ret.Breadcrumb = []*BlockPath{}
	default:
		return PositionResult{}, errors.New("block_position_mode")
	}
	tree, e := s.Load(ctx, id)
	if e != nil {
		return PositionResult{}, e
	}
	if e = ctx.Err(); e != nil {
		return PositionResult{}, e
	}
	if tree == nil {
		return ret, nil
	}
	if tree.Root == nil {
		return PositionResult{}, errors.New("block_position_invalid_tree")
	}
	if q.Mode == "indexes" {
		idx := 0
		indexes := map[string]int{}
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if !enter {
				return ast.WalkContinue
			}
			if !n.IsChildBlockOf(tree.Root, 1) {
				if n.IsBlock() {
					indexes[n.ID] = idx
				}
				return ast.WalkContinue
			}
			idx++
			indexes[n.ID] = idx
			return ast.WalkContinue
		})
		for _, id := range q.IDs {
			ret.Indexes[id] = indexes[id]
		}
	} else {
		var node *ast.Node
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if ctx.Err() != nil {
				return ast.WalkStop
			}
			if enter && n.ID == id {
				node = n
				return ast.WalkStop
			}
			return ast.WalkContinue
		})
		if e = ctx.Err(); e != nil {
			return PositionResult{}, e
		}
		if node == nil {
			return ret, nil
		}
		if q.Mode == "breadcrumb" {
			ret.Breadcrumb, e = BreadcrumbFromNode(ctx, node, tree.Box, tree.Path, q, s)
			if e != nil {
				return PositionResult{}, e
			}
		} else {
			top := node
			for top.Parent != nil && top.Parent.Type != ast.NodeDocument {
				if e = ctx.Err(); e != nil {
					return PositionResult{}, e
				}
				top = top.Parent
			}
			ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
				if ctx.Err() != nil {
					return ast.WalkStop
				}
				if !enter || !n.IsChildBlockOf(tree.Root, 1) {
					return ast.WalkContinue
				}
				ret.Index++
				if n.ID == top.ID {
					return ast.WalkStop
				}
				return ast.WalkContinue
			})
		}
	}
	if e = ctx.Err(); e != nil {
		return PositionResult{}, e
	}
	return ret, nil
}

func cleanBreadcrumbHTML(text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	return html.EscapeString(html.UnescapeString(text))
}
func positionSubtype(n *ast.Node) (string, error) {
	switch n.Type {
	case ast.NodeList, ast.NodeListItem:
		if n.ListData == nil {
			return "", errors.New("block_position_invalid_list")
		}
		switch n.ListData.Typ {
		case 0:
			return "u", nil
		case 1:
			return "o", nil
		case 3:
			return "t", nil
		}
	case ast.NodeHeading:
		if n.HeadingLevel >= 1 && n.HeadingLevel <= 6 {
			return "h" + strconv.Itoa(n.HeadingLevel), nil
		}
	}
	return "", nil
}

// Used by both ordinary navigation and the original embed/backlink helper.
// box/path are explicit: reads never annotate/move caller-owned AST nodes.
func BreadcrumbFromNode(ctx context.Context, node *ast.Node, box, path string, q PositionRequest, s PositionServices) (ret []*BlockPath, err error) {
	ret = []*BlockPath{}
	if ctx == nil {
		return nil, ErrPositionDependency
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if node == nil {
		return ret, nil
	}
	if s.Notebook == nil || s.RootPath == nil || s.AVName == nil || s.Text == nil {
		return nil, ErrPositionDependency
	}
	q.ExcludeTypes = slices.Clone(q.ExcludeTypes)
	boxName, found, e := s.Notebook(ctx, box)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	if !found {
		return ret, nil
	}
	hPath, e := s.RootPath(ctx, box, path)
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	text := func(n *ast.Node) (string, error) {
		if e := ctx.Err(); e != nil {
			return "", e
		}
		v, e := s.Text(ctx, n, slices.Clone(q.ExcludeTypes), true)
		if e != nil {
			return "", e
		}
		return gulu.Str.SubStr(v, 1024), ctx.Err()
	}
	headingLevel := 16
	for parent := node; parent != nil; parent = parent.Parent {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if parent.ID == "" {
			continue
		}
		fc := anchor.FirstLeaf(parent)
		name := parent.IALAttr("name")
		switch parent.Type {
		case ast.NodeDocument:
			name = boxName + hPath
		case ast.NodeAttributeView:
			name, e = s.AVName(ctx, parent.AttributeViewID)
			if e != nil {
				return nil, e
			}
		default:
			if name == "" {
				target := parent
				if parent.Type == ast.NodeListItem || parent.Type == ast.NodeList || parent.Type == ast.NodeSuperBlock || parent.Type == ast.NodeBlockquote {
					target = fc
				}
				name, e = text(target)
				if e != nil {
					return nil, e
				}
			}
			if parent.Type == ast.NodeHeading {
				headingLevel = parent.HeadingLevel
			}
		}
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		add := true
		if parent.Type == ast.NodeList || parent.Type == ast.NodeSuperBlock || parent.Type == ast.NodeBlockquote {
			add = parent == node
		}
		if parent.Type == ast.NodeParagraph && parent.Parent != nil && parent.Parent.Type == ast.NodeListItem && parent.Next == nil && (parent.Previous == nil || parent.Previous.Type == ast.NodeTaskListItemMarker) {
			add = false
		}
		if parent.Type == ast.NodeListItem && name == "" {
			name, e = text(fc)
			if e != nil {
				return nil, e
			}
		}
		name = cleanBreadcrumbHTML(strings.ReplaceAll(name, editor.Caret, ""))
		if !q.Embed {
			if parent == node {
				name = ""
			}
		} else if parent.Type != ast.NodeDocument {
			if !(q.HeadingMode == 2 && parent.Type == ast.NodeHeading && parent == node) {
				name = ""
			}
		}
		if add {
			sub, e := positionSubtype(parent)
			if e != nil {
				return nil, e
			}
			ret = append([]*BlockPath{{ID: parent.ID, Name: name, Type: parent.Type.String(), SubType: sub}}, ret...)
		}
		for prev := parent.Previous; prev != nil; prev = prev.Previous {
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			b := prev
			if prev.Type == ast.NodeSuperBlock {
				headings := prev.ChildrenByType(ast.NodeHeading)
				if len(headings) > 0 {
					b = headings[len(headings)-1]
				}
			}
			if b.Type == ast.NodeHeading && headingLevel > b.HeadingLevel {
				if b.ParentIs(ast.NodeListItem) {
					continue
				}
				name, e = text(b)
				if e != nil {
					return nil, e
				}
				name = cleanBreadcrumbHTML(name)
				sub, e := positionSubtype(b)
				if e != nil {
					return nil, e
				}
				ret = append([]*BlockPath{{ID: b.ID, Name: name, Type: b.Type.String(), SubType: sub}}, ret...)
				headingLevel = b.HeadingLevel
			}
		}
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return ret, nil
}
