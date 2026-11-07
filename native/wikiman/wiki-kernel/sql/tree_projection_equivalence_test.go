package sql

import (
	"bytes"
	"context"
	"fmt"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/util"
)

// The old original walk is retained as a differential oracle here. Actual
// index/upsert callers use fullRows -> ProjectTree, not a second new algorithm.
func TestFullTreeProjectionPreservesOriginalRows(t *testing.T) {
	settings := nativeparser.Defaults()
	makeTree := func() *parse.Tree {
		engine := nativeparser.NewIndex(settings)
		tree := parse.Parse("fixture", []byte("# Heading\n\ntext **bold**\n\n- first\n- second\n\n| A | B |\n| - | - |\n| x | y |\n\n```go\nfmt.Println(1)\n```\n"), engine.ParseOptions)
		tree.ID = "20261022000000-doc0001"
		tree.Root.ID = tree.ID
		tree.Root.SetIALAttr("id", tree.ID)
		tree.Root.SetIALAttr("updated", "20261022000000")
		tree.Box = "20261022000000-box0001"
		tree.Path = "/nested/root.docj"
		tree.HPath = "/Parent/Title"
		tree.Root.SetIALAttr("title", "Title")
		tree.Root.SetIALAttr("tags", "a, b & c")
		i := 0
		var markers []*ast.Node
		ast.Walk(tree.Root, func(n *ast.Node, e bool) ast.WalkStatus {
			if e {
				if n.Type == ast.NodeKramdownBlockIAL {
					markers = append(markers, n)
					return ast.WalkSkipChildren
				}
				n.Box = tree.Box
				n.Path = tree.Path
				if n.IsBlock() && n != tree.Root {
					i++
					n.ID = fmt.Sprintf("20261022000000-%07d", i)
					n.SetIALAttr("id", n.ID)
					n.SetIALAttr("updated", "20261022000000")
				}
			}
			return ast.WalkContinue
		})
		for _, n := range markers {
			n.Unlink()
		}
		p := tree.Root.FirstChild.Next
		p.SetIALAttr("custom-label", "some & value")
		p.AppendChild(&ast.Node{Type: ast.NodeTextMark, TextMarkType: "tag", TextMarkTextContent: "topic"})
		image := &ast.Node{Type: ast.NodeImage}
		image.AppendChild(&ast.Node{Type: ast.NodeLinkText, Tokens: []byte("caption")})
		image.AppendChild(&ast.Node{Type: ast.NodeLinkDest, Tokens: []byte("assets/image.png")})
		p.AppendChild(image)
		media := &ast.Node{Type: ast.NodeVideo, ID: "20261022000000-vid0001", Tokens: []byte(`<video src="assets/movie.mp4"></video>`), Box: tree.Box, Path: tree.Path}
		tree.Root.AppendChild(media)
		return tree
	}
	oldOCR, oldPath := util.TesseractEnabled, util.DataDir
	util.TesseractEnabled = false
	util.DataDir = t.TempDir()
	defer func() { util.TesseractEnabled, util.DataDir = oldOCR, oldPath }()
	p := newTreeProjection(settings)
	// Use a single matching root/tree for the oracle, as the actual original code does.
	original := makeTree()
	blocks, spans, assets, attrs := p.fromTree(original.Root, original)
	rows, err := indexdb.ProjectTree(context.Background(), makeTree(), nativeparser.NewIndex(settings), indexdb.TreeServices{Content: legacyContentServices(), IndexAssetPath: indexAssetPath, AssetHash: func(context.Context, string, string, string) (string, error) { return "", nil }, PrepareOCR: func(context.Context, *ast.Node) error { return nil }}, func(context.Context, string) (indexdb.BlockLocation, bool, error) {
		return indexdb.BlockLocation{}, false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var old indexdb.TreeRecords
	for _, v := range blocks {
		old.Blocks = append(old.Blocks, indexdb.BlockRecord(*v))
	}
	for _, v := range spans {
		x := indexdb.SpanRecord(*v)
		x.ID = ""
		old.Spans = append(old.Spans, x)
	}
	for _, v := range assets {
		x := indexdb.AssetRecord(*v)
		x.ID = ""
		old.Assets = append(old.Assets, x)
	}
	for _, v := range attrs {
		x := indexdb.AttributeRecord(*v)
		x.ID = ""
		old.Attributes = append(old.Attributes, x)
	}
	for i := range rows.Spans {
		rows.Spans[i].ID = ""
	}
	for i := range rows.Assets {
		rows.Assets[i].ID = ""
	}
	for i := range rows.Attributes {
		rows.Attributes[i].ID = ""
	}
	if !reflect.DeepEqual(old.Blocks, rows.Blocks) || !reflect.DeepEqual(old.Spans, rows.Spans) || !reflect.DeepEqual(old.Assets, rows.Assets) || !reflect.DeepEqual(old.Attributes, rows.Attributes) {
		t.Fatalf("projection changed\noriginal=%#v\nnew=%#v", old, rows)
	}
}

func TestContextAssetEtagPreservesOriginalSmallAndMultiBlockHashes(t *testing.T) {
	for _, size := range []int{0, 17, 1 << 22, (1 << 22) + 33} {
		data := bytes.Repeat([]byte("x"), size)
		want, err := util.GetEtagByHandle(bytes.NewReader(data), int64(size))
		if err != nil {
			t.Fatal(err)
		}
		got, err := indexdb.AssetEtag(context.Background(), bytes.NewReader(data), int64(size))
		if err != nil || got != want {
			t.Fatal(size, got, want, err)
		}
	}
}
