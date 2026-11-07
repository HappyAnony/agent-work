package doclife

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/docview"
)

func swapTestTree() (*parse.Tree, *ast.Node, *ast.Node) {
	root := &ast.Node{Type: ast.NodeDocument, ID: "root"}
	list := &ast.Node{Type: ast.NodeList, ListData: &ast.ListData{Typ: 1}, ID: "list"}
	li := &ast.Node{Type: ast.NodeListItem, ListData: &ast.ListData{Typ: 1}, ID: "item"}
	p := &ast.Node{Type: ast.NodeParagraph, ID: "paragraph"}
	li.AppendChild(p)
	list.AppendChild(li)
	root.AppendChild(list)
	def := &ast.Node{Type: ast.NodeParagraph, ID: "definition"}
	root.AppendChild(def)
	return &parse.Tree{Root: root, ID: "root"}, p, def
}
func TestReferenceSwapListRetainsTypeAfterReparenting(t *testing.T) {
	tree, p, def := swapTestTree()
	changed, e := SwapReferences(context.Background(), tree, tree, p.ID, def.ID, false, ReferenceSwapPorts{NewID: ast.NewNodeID, Updated: func(n *ast.Node) { RefreshUpdatedAt(n, "20261104000000") }})
	if e != nil || !changed {
		t.Fatal(changed, e)
	}
	if def.Parent.Type != ast.NodeListItem || def.Parent.ListData.Typ != 1 || p.Parent.Parent.Type != ast.NodeList || p.Parent.Parent.ListData.Typ != 1 {
		t.Fatal("lost original list type")
	}
	if docview.GetNodeInTree(tree, p.ID) != p || docview.GetNodeInTree(tree, def.ID) != def {
		t.Fatal("lost nodes")
	}
}
func TestReferenceSwapRejectsCyclesAndTransferPreservesStaticAnchor(t *testing.T) {
	tree, p, _ := swapTestTree()
	if changed, e := SwapReferences(context.Background(), tree, tree, p.ID, "list", false, ReferenceSwapPorts{NewID: ast.NewNodeID, Updated: func(*ast.Node) {}}); e == nil || changed {
		t.Fatal(changed, e)
	}
	for _, kind := range []string{"d", "s"} {
		p.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "block-ref", TextMarkBlockRefID: "old", TextMarkBlockRefSubtype: kind, TextMarkTextContent: "old text"})
	}
	changed, e := TransferReferences(context.Background(), tree, p.ID, "old", "new", "new text")
	if e != nil || !changed || p.FirstChild.TextMarkTextContent != "new text" || p.LastChild.TextMarkTextContent != "old text" {
		t.Fatal(changed, e)
	}
}
