package blockview

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
	"wiki-kernel/internal/nativeparser"
)

func TestOutlineHierarchyPreviewFoldStorageAndInvalidInput(t *testing.T) {
	makeTree := func() *parse.Tree {
		root := &ast.Node{ID: "root", Type: ast.NodeDocument}
		root.SetIALAttr("title", "Doc & title")
		for i, v := range []struct {
			id    string
			level int
		}{{"first", 1}, {"nested", 3}, {"last", 1}} {
			n := &ast.Node{ID: v.id, Type: ast.NodeHeading, HeadingLevel: v.level}
			n.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(v.id)})
			if i == 0 {
				n.SetIALAttr("style", "font-size: 20px;")
			}
			root.AppendChild(n)
		}
		return &parse.Tree{Root: root, Box: "box", Path: "/root.docj"}
	}
	parser := nativeparser.NewEditor(nativeparser.EditorOptions{})
	out, e := OutlineView(context.Background(), makeTree(), parser, false, nil)
	if e != nil || len(out) != 2 || out[0].ID != "first" || out[0].Folded || len(out[0].Blocks) != 1 || out[0].Blocks[0].Depth != 1 || out[0].Blocks[0].Folded || out[0].SubType != "h1" || !strings.Contains(out[0].Name, "font-size: inherit;") {
		t.Fatal(out, e)
	}
	out, e = OutlineView(context.Background(), makeTree(), parser, true, map[string]interface{}{"expandIds": []interface{}{"root", "nested"}})
	if e != nil || len(out) != 3 || out[0].ID != "root" || out[0].Folded || out[1].ID != "first" || !out[1].Folded || len(out[1].Blocks) != 1 || out[1].Blocks[0].Folded {
		t.Fatal(out, e)
	}
	if _, e = OutlineView(context.Background(), makeTree(), parser, false, map[string]interface{}{"expandIds": "bad"}); e == nil {
		t.Fatal("invalid storage accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = OutlineView(ctx, makeTree(), parser, false, nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
