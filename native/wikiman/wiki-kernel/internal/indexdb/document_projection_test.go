package indexdb

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
	"wiki-kernel/internal/nativeparser"
)

func TestDocumentProjectionOriginalRulesNoBodyTraversal(t *testing.T) {
	id := "20261030120000-title01"
	ial := [][]string{{"title", "A\u200b &amp; B"}, {"name", "Name &amp; alias"}, {"alias", "Alias"}, {"tags", "one, two"}, {"updated", "20261030"}, {"custom-key", "kept"}, {"id", id}}
	root := &ast.Node{ID: id, Type: ast.NodeDocument, KramdownIAL: ial}
	tree := &parse.Tree{ID: id, Root: root, Box: "box", Path: "/doc.docj", HPath: "/Title"}
	root.FirstChild = &ast.Node{Type: ast.NodeAttributeView, AttributeViewID: "never-read"}
	before := make([][]string, len(ial))
	for i, kv := range ial {
		before[i] = append([]string(nil), kv...)
	}
	parser := nativeparser.NewIndex(nativeparser.Defaults())
	row, err := ProjectDocumentBlock(context.Background(), tree, parser)
	if err != nil || row.Content != "A & B" || row.Name != "Name & alias" || row.Tag != "#one# #two#" || row.Type != "d" || row.ID != id || row.Markdown != "" || row.Created != id[:14] {
		t.Fatal(row, err)
	}
	if !reflect.DeepEqual(root.KramdownIAL, before) || root.FirstChild.AttributeViewID != "never-read" {
		t.Fatal("root/body mutated")
	}
	// The original root branch, isolated here with its explicit options, is the parity oracle.
	copyRoot := &ast.Node{ID: id, Type: ast.NodeDocument, KramdownIAL: make([][]string, len(ial))}
	for i, kv := range ial {
		copyRoot.KramdownIAL[i] = append([]string(nil), kv...)
	}
	copyTree := *tree
	copyTree.Root = copyRoot
	p := &treeProjector{ctx: context.Background(), parser: parser}
	want, _ := p.buildBlockFromNode(copyRoot, &copyTree)
	if !reflect.DeepEqual(row, want) {
		t.Fatal("document branch changed", row, want)
	}
}
func TestDocumentProjectionMalformedAndCanceledInputs(t *testing.T) {
	id := "20261030120000-title01"
	parser := nativeparser.NewIndex(nativeparser.Defaults())
	makeTree := func() *parse.Tree { return &parse.Tree{ID: id, Root: &ast.Node{ID: id, Type: ast.NodeDocument}} }
	for _, kind := range []string{"nil-context", "nil-tree", "nil-root", "wrong-type", "identity", "short-id", "bad-ial", "nil-parser", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			tree := makeTree()
			engine := parser
			switch kind {
			case "nil-context":
				ctx = nil
			case "nil-tree":
				tree = nil
			case "nil-root":
				tree.Root = nil
			case "wrong-type":
				tree.Root.Type = ast.NodeParagraph
			case "identity":
				tree.ID = "different"
			case "short-id":
				tree.ID = "short"
				tree.Root.ID = "short"
			case "bad-ial":
				tree.Root.KramdownIAL = [][]string{{"only-key"}}
			case "nil-parser":
				engine = nil
			case "canceled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			row, err := ProjectDocumentBlock(ctx, tree, engine)
			if err == nil || row != nil {
				t.Fatal(row, err)
			}
			if kind == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}
