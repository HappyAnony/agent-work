//go:build fts5

package treenode

import (
	"context"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalBlockTreeFacadeUsesSharedOwnedQueriesAndWrites(t *testing.T) {
	bundle, err := indexdb.OpenMemory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	previous := db
	defer func() { db = previous }()
	db, _ = bundle.Database(indexdb.BlockTree)
	root := &ast.Node{Type: ast.NodeDocument, ID: "root'quoted"}
	p := &ast.Node{Type: ast.NodeParagraph, ID: "paragraph"}
	root.AppendChild(p)
	tree := &parse.Tree{ID: root.ID, Root: root, Box: "box", Path: "/a.docj", HPath: "/Title"}
	IndexBlockTree(tree)
	if CountBlocks() != 2 || CountTrees() != 1 || !ExistBlockTrees([]string{root.ID})[root.ID] {
		t.Fatal("original wrapper diverged")
	}
	if got := GetBlockTreeRootByHPath("box", "/Ti\u200btle"); got == nil || got.ID != root.ID {
		t.Fatal(got)
	}
	tree.Path = "/renamed.docj"
	tree.HPath = "/Renamed"
	SetBlockTreePath(tree)
	if got := GetBlockTree("paragraph"); got == nil || got.Path != tree.Path {
		t.Fatal(got)
	}
	UpsertBlockTree(tree)
	if len(GetBlockTreesByPathPrefix("/renamed")) != 2 || CountBlockTreesByPathPrefix("/renamed") != 2 {
		t.Fatal("prefix behavior changed")
	}
	if ids := RemoveBlockTreesByBoxID("box"); len(ids) != 2 || CountBlocks() != 0 {
		t.Fatal(ids)
	}
}
