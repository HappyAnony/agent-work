package indexdb

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/nativeparser"
)

func TestFullTreeProjectionBorrowedServicePanicReturnsNoRows(t *testing.T) {
	root := &ast.Node{Type: ast.NodeDocument, ID: "20261022000000-doc0001"}
	p := &ast.Node{Type: ast.NodeParagraph, ID: "20261022000000-par0001"}
	root.AppendChild(p)
	image := &ast.Node{Type: ast.NodeImage}
	image.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte("assets/a.png")})
	p.AppendChild(image)
	rows, err := ProjectTree(context.Background(), &parse.Tree{ID: root.ID, Root: root, Box: "box", Path: "/doc.docj"}, nativeparser.NewIndex(nativeparser.Defaults()), TreeServices{PrepareOCR: func(context.Context, *ast.Node) error { panic("synthetic reader panic") }}, func(context.Context, string) (BlockLocation, bool, error) { return BlockLocation{}, false, nil })
	if !errors.Is(err, ErrProjection) || len(rows.Blocks) != 0 || len(rows.Assets) != 0 {
		t.Fatal(rows, err)
	}
}
