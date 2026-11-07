//go:build fts5

package indexdb

import (
	"context"
	"database/sql"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
)

func registryTree() *parse.Tree {
	root := &ast.Node{Type: ast.NodeDocument, ID: "root'quoted"}
	list := &ast.Node{Type: ast.NodeList, ID: "list", ListData: &ast.ListData{}}
	p := &ast.Node{Type: ast.NodeParagraph, ID: "paragraph"}
	root.AppendChild(list)
	list.AppendChild(p)
	for _, n := range []*ast.Node{root, list, p} {
		n.SetIALAttr("updated", "1")
	}
	return &parse.Tree{ID: root.ID, Root: root, Box: "box", Path: "/nested.docj", HPath: "/Title"}
}
func TestOriginalBlockTreeQueriesIncrementalAndSnapshotSemantics(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(BlockTree)
	q := BlockTrees{DB: db}
	tree := registryTree()
	if err = q.Index(ctx, tree); err != nil {
		t.Fatal(err)
	}
	for _, f := range []BlockTreeFilter{{Kind: "all"}, {Kind: "box", Value: "box"}, {Kind: "root", Value: tree.ID}, {Kind: "prefix", Value: "/nested"}} {
		rows, e := q.Query(ctx, f)
		if e != nil || len(rows) != 3 {
			t.Fatal(f, rows, e)
		}
	}
	for _, f := range []BlockTreeFilter{{Kind: "id", Value: tree.ID}, {Kind: "root-path", BoxID: "box", Value: tree.Path}, {Kind: "root-hpath", BoxID: "box", Value: "/Ti\u200btle"}, {Kind: "preferred", BoxID: "box", Value: "/Title", ParentID: ""}, {Kind: "type", Value: "d"}} {
		rows, e := q.Query(ctx, f)
		if e != nil || len(rows) != 1 || rows[0].ID != tree.ID {
			t.Fatal(f, rows, e)
		}
	}
	ids := make([]string, 1025)
	for i := range ids {
		ids[i] = "none"
	}
	ids[512] = tree.ID
	rows, err := q.Query(ctx, BlockTreeFilter{Kind: "ids", IDs: ids})
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	tree.Root.FirstChild.SetIALAttr("updated", "2")
	tree.Root.FirstChild.FirstChild.SetIALAttr("updated", "2")
	if err = q.Upsert(ctx, tree); err != nil {
		t.Fatal(err)
	}
	rows, err = q.Query(ctx, BlockTreeFilter{Kind: "id", Value: "paragraph"})
	if err != nil || len(rows) != 2 {
		t.Fatal("original descendant repeat changed", rows, err)
	}
	tree.Root.FirstChild.FirstChild.Unlink()
	if err = q.Upsert(ctx, tree); err != nil {
		t.Fatal(err)
	}
	rows, _ = q.Query(ctx, BlockTreeFilter{Kind: "id", Value: "paragraph"})
	if len(rows) != 2 {
		t.Fatal("incremental upsert silently changed delete contract")
	}
	if err = q.Replace(ctx, tree.ID, ProjectBlockTree(tree)); err != nil {
		t.Fatal(err)
	}
	rows, _ = q.Query(ctx, BlockTreeFilter{Kind: "id", Value: "paragraph"})
	if len(rows) != 0 {
		t.Fatal("snapshot retained deleted block")
	}
	if _, err = db.Exec("CREATE TRIGGER fail_registry BEFORE INSERT ON blocktrees WHEN NEW.id='bad' BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	bad := []BlockTreeRecord{{ID: "bad", RootID: tree.ID}}
	if err = q.Replace(ctx, "", bad); err == nil {
		t.Fatal("failure missing")
	}
	n, err := q.Count(ctx, BlockTreeFilter{Kind: "all"})
	if err != nil || n != 2 {
		t.Fatal("partial snapshot published", n, err)
	}
	removed, err := q.Remove(ctx, BlockTreeFilter{Kind: "box", Value: "box"})
	if err != nil || len(removed) != 2 {
		t.Fatal(removed, err)
	}
	if _, err = q.Query(ctx, BlockTreeFilter{Kind: "unsafe", Value: strings.Repeat("x", 5)}); err == nil {
		t.Fatal("arbitrary SQL filter")
	}
	if err = Transaction(ctx, db, func(tx *sql.Tx) error { return insertBlockTrees(ctx, tx, []BlockTreeRecord{}) }); err != nil {
		t.Fatal(err)
	}
}
func TestBlockTreeUsesPhysicalParentNotHeadingParent(t *testing.T) {
	tree := registryTree()
	root := tree.Root
	h := &ast.Node{Type: ast.NodeHeading, ID: "heading", HeadingLevel: 1}
	p := &ast.Node{Type: ast.NodeParagraph, ID: "sibling"}
	root.AppendChild(h)
	root.AppendChild(p)
	rows := ProjectBlockTree(tree)
	last := rows[len(rows)-1]
	if last.ParentID != root.ID {
		t.Fatal(last)
	}
}
