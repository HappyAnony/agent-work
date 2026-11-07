// SPDX-License-Identifier: AGPL-3.0-or-later
package doclife

import (
	"bytes"
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/html"
	"github.com/88250/lute/parse"
	"strings"
	"text/template"
	"time"
	"wiki-kernel/internal/anchor"
)

type creationWriter struct {
	ctx context.Context
	bytes.Buffer
}

func (w *creationWriter) Write(p []byte) (int, error) {
	if e := w.ctx.Err(); e != nil {
		return 0, e
	}
	if len(p)+w.Len() > 4<<20 {
		return 0, errors.New("document_creation_template_output_limit")
	}
	return w.Buffer.Write(p)
}
func RenderCreationText(ctx context.Context, source string, action bool, funcs template.FuncMap, data interface{}) (string, error) {
	if ctx == nil || funcs == nil {
		return "", ErrDependency
	}
	if e := ctx.Err(); e != nil {
		return "", e
	}
	t := template.New("").Funcs(funcs)
	if action {
		t = t.Delims(".action{", "}")
	}
	t, e := t.Parse(source)
	if e != nil {
		return "", e
	}
	w := &creationWriter{ctx: ctx}
	if e = t.Execute(w, data); e != nil {
		return "", e
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	return w.String(), nil
}

type TemplateBodyServices struct {
	NewID     func() string
	Now       func() time.Time
	RefText   func(context.Context, string) (string, error)
	CloneAV   func(context.Context, string) (string, error)
	PreviewAV func(context.Context, string, string) ([]string, error)
	Warning   func(error)
}

// Original daily-template body adaptation, including the original normalize
// rules, empty container repairs, refs, cloned AVs, heading-fold and icon IAL.
func RenderCreationBody(ctx context.Context, text string, parser *lute.Lute, s TemplateBodyServices) (*parse.Tree, string, error) {
	if ctx == nil || parser == nil || s.NewID == nil || s.Now == nil {
		return nil, "", ErrDependency
	}
	t := parse.Parse("", []byte(text), parser.ParseOptions)
	if _, _, _, e := NormalizeCreationTemplate(ctx, t, s); e != nil {
		return nil, "", e
	}
	var appendChild, unlinks []*ast.Node
	var err error
	ast.Walk(t.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
		if ctx.Err() != nil || err != nil {
			return ast.WalkStop
		}
		if !enter {
			return ast.WalkContinue
		}
		if n.ID != "" {
			n.ID = s.NewID()
			if !ast.IsNodeIDPattern(n.ID) {
				err = errors.New("document_creation_invalid_id")
				return ast.WalkStop
			}
			n.SetIALAttr("id", n.ID)
			n.RemoveIALAttr("custom-avs")
			n.SetIALAttr("updated", s.Now().Format("20060102150405"))
		}
		if n.Type == ast.NodeListItem && (n.FirstChild == nil || n.ListData != nil && n.ListData.Typ == 3 && (n.FirstChild.Next == nil || n.FirstChild.Next.Type == ast.NodeKramdownBlockIAL)) || n.Type == ast.NodeBlockquote && n.FirstChild != nil && n.FirstChild.Next != nil && n.FirstChild.Next.Type == ast.NodeKramdownBlockIAL {
			appendChild = append(appendChild, n)
		}
		if (n.IsTextMarkType("block-ref") || n.Type == ast.NodeBlockRef) && n.Text() == "" {
			id := n.TextMarkBlockRefID
			if n.Type == ast.NodeBlockRef {
				if child := n.ChildByType(ast.NodeBlockRefID); child != nil {
					id = child.TokensStr()
				}
			}
			if id != "" {
				if s.RefText == nil {
					err = ErrDependency
					return ast.WalkStop
				}
				var text string
				text, err = s.RefText(ctx, id)
				text = strings.TrimSpace(text)
				if err != nil {
					return ast.WalkStop
				}
				if text == "" {
					unlinks = append(unlinks, n)
				} else {
					err = anchor.SetText(n, text, func(string, string) error { return nil })
				}
			}
		} else if n.IsTextMarkType("inline-math") && n.ParentIs(ast.NodeTableCell) {
			n.TextMarkInlineMathContent = strings.ReplaceAll(n.TextMarkInlineMathContent, "|", "&#124;")
		}
		if n.Type == ast.NodeAttributeView {
			if s.PreviewAV != nil {
				var names []string
				names, err = s.PreviewAV(ctx, n.AttributeViewID, n.IALAttr("custom-sy-av-view"))
				if err != nil {
					return ast.WalkStop
				}
				aligns := make([]int, len(names))
				table := &ast.Node{Type: ast.NodeTable, TableAligns: aligns}
				head := &ast.Node{Type: ast.NodeTableHead}
				row := &ast.Node{Type: ast.NodeTableRow, TableAligns: aligns}
				table.AppendChild(head)
				head.AppendChild(row)
				for _, name := range names {
					cell := &ast.Node{Type: ast.NodeTableCell}
					cell.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(name)})
					row.AppendChild(cell)
				}
				n.InsertBefore(table)
				unlinks = append(unlinks, n)
				return ast.WalkContinue
			}
			if s.CloneAV == nil {
				err = ErrDependency
				return ast.WalkStop
			}
			n.AttributeViewID, err = s.CloneAV(ctx, n.AttributeViewID)
		}
		if n.IALAttr("heading-fold") == "1" {
			n.SetIALAttr("status", "temp")
		}
		return ast.WalkContinue
	})
	if err = creationErr(ctx, err); err != nil {
		return nil, "", err
	}
	for _, n := range appendChild {
		if n.Type == ast.NodeBlockquote {
			n.FirstChild.InsertAfter(templateParagraph(s))
		} else {
			n.AppendChild(templateParagraph(s))
		}
	}
	for _, n := range unlinks {
		n.Unlink()
	}
	if icon := t.Root.IALAttr("icon"); icon != "" {
		t.Root.SetIALAttr("icon", html.UnescapeString(icon))
	}
	dom := parser.Tree2BlockDOM(t, parser.RenderOptions)
	if e := ctx.Err(); e != nil {
		return nil, "", e
	}
	return t, dom, nil
}
