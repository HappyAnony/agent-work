// SPDX-License-Identifier: AGPL-3.0-or-later
package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/88250/lute/render"
	"wiki-kernel/internal/docview"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/markup"
)

type ChildBlock struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	SubType  string `json:"subType,omitempty"`
	Content  string `json:"content,omitempty"`
	Markdown string `json:"markdown,omitempty"`
}
type BlocksRequest struct {
	Mode, ID   string
	Tail       bool
	Limit      int
	RemoveFold bool
}
type BlocksResult struct {
	Block    *indexdb.BlockRecord
	Children []*ChildBlock
	IDs      []string
	Text     string
}
type BlocksServices struct {
	Render  func(context.Context, []*ast.Node, *lute.Lute) (string, error)
	Load    func(context.Context, string) (*parse.Tree, error)
	Parser  *lute.Lute
	Content indexdb.ContentServices
	Missing error
}

// Blocks operates on a detached, owner-loaded tree. It has no implicit loader,
// parser configuration, index connection, cache, or source writer.
func Blocks(ctx context.Context, q BlocksRequest, s BlocksServices) (out BlocksResult, err error) {
	if ctx == nil || s.Load == nil || s.Parser == nil || s.Parser.RenderOptions == nil || s.Missing == nil {
		return out, errors.New("block_read_dependencies_unbound")
	}
	if err = ctx.Err(); err != nil {
		return
	}
	if q.ID == "" {
		return out, nil
	}
	tree, err := s.Load(ctx, q.ID)
	if err != nil {
		return out, err
	}
	if tree == nil {
		return out, s.Missing
	}
	node, err := relationNode(ctx, tree, q.ID)
	if err != nil {
		return out, err
	}
	if node == nil {
		return out, s.Missing
	}
	parser := *s.Parser
	options := *s.Parser.RenderOptions
	parser.RenderOptions = &options
	switch q.Mode {
	case "block":
		out.Block, err = indexdb.ProjectBlock(ctx, tree, node, &parser, s.Content)
	case "children":
		out.Children = []*ChildBlock{}
		var children []*ast.Node
		if node.Type == ast.NodeHeading {
			children = docview.HeadingChildren(node)
		} else if node.IsContainerBlock() {
			for c := node.FirstChild; c != nil; c = c.Next {
				if c.IsBlock() {
					children = append(children, c)
				}
			}
		}
		for i := 0; i < len(children); i++ {
			if err = ctx.Err(); err != nil {
				return BlocksResult{}, err
			}
			j := i
			if q.Tail {
				j = len(children) - 1 - i
			}
			n := children[j]
			b, e := indexdb.ProjectBlock(ctx, tree, n, &parser, s.Content)
			if e != nil {
				return BlocksResult{}, e
			}
			if b == nil {
				continue
			}
			out.Children = append(out.Children, &ChildBlock{ID: n.ID, Type: b.Type, SubType: b.SubType, Content: b.Content, Markdown: b.Markdown})
			if q.Tail && q.Limit > 0 && len(out.Children) >= q.Limit {
				break
			}
		}
	case "heading-ids", "heading-dom":
		if node.Type != ast.NodeHeading {
			return out, nil
		}
		children := docview.HeadingChildren(node)
		if q.Mode == "heading-ids" {
			for _, n := range children {
				if err = ctx.Err(); err != nil {
					return BlocksResult{}, err
				}
				out.IDs = append(out.IDs, n.ID)
			}
			break
		}
		if q.RemoveFold {
			node.RemoveIALAttr("fold")
			node.RemoveIALAttr("heading-fold")
		}
		for _, c := range children {
			ast.Walk(c, func(n *ast.Node, enter bool) ast.WalkStatus {
				if ctx.Err() != nil {
					return ast.WalkStop
				}
				if enter && q.RemoveFold {
					n.RemoveIALAttr("fold")
					n.RemoveIALAttr("heading-fold")
				}
				return ast.WalkContinue
			})
			if q.RemoveFold {
				c.RemoveIALAttr("parent-heading")
			} else {
				c.SetIALAttr("parent-heading", q.ID)
			}
		}
		if s.Render == nil {
			return BlocksResult{}, errors.New("block_render_dependencies_unbound")
		}
		out.Text, err = s.Render(ctx, append([]*ast.Node{node}, children...), &parser)
	case "kramdown", "md":
		if err = markup.AddBlockIALNodes(ctx, tree, false); err != nil {
			return BlocksResult{}, err
		}
		root := &ast.Node{Type: ast.NodeDocument}
		if node.Next != nil && node.Next.Type == ast.NodeKramdownBlockIAL {
			root.AppendChild(node.Next)
		}
		root.PrependChild(node)
		tree.Root = root
		if q.Mode == "md" {
			parser.SetPreventEncodeLinkSpace(true)
			out.Text, err = lute.ProtyleExportMdNodeSync(root, parser.ParseOptions, parser.RenderOptions)
		} else {
			out.Text = string(render.NewFormatRenderer(tree, parser.RenderOptions).Render())
		}
	default:
		return out, errors.New("block_read_mode_invalid")
	}
	if err != nil {
		return BlocksResult{}, err
	}
	if err = ctx.Err(); err != nil {
		return BlocksResult{}, err
	}
	return
}
