//go:build fts5

package sql

import (
	"context"
	stdsql "database/sql"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
	"wiki-kernel/util"
)

// Exercise the actual original AST -> index/upsert path, not only the extracted
// row writer. The global DB is deliberately another owner's empty database.
func TestOriginalTreeUpsertUsesWritingOwnerNotGlobalDatabase(t *testing.T) {
	ctx := context.Background()
	a, err := indexdb.OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := indexdb.OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	owner, _ := a.Database(indexdb.Main)
	other, _ := b.Database(indexdb.Main)
	oldDB, oldCase, oldIgnore, oldLines, oldMarkdown := db, caseSensitive, IndexIgnoreCached, indexIgnore, util.MarkdownSettings
	defer func() {
		db, caseSensitive, IndexIgnoreCached, indexIgnore, util.MarkdownSettings = oldDB, oldCase, oldIgnore, oldLines, oldMarkdown
		ClearCache()
	}()
	db, caseSensitive, IndexIgnoreCached, indexIgnore = other, false, true, nil
	settings := nativeparser.Defaults()
	util.MarkdownSettings = &settings
	root := &ast.Node{Type: ast.NodeDocument, ID: "20261021120000-root001"}
	root.SetIALAttr("id", root.ID)
	root.SetIALAttr("title", "Original title")
	root.SetIALAttr("updated", "20261021120000")
	para := &ast.Node{Type: ast.NodeParagraph, ID: "20261021120001-para001"}
	para.SetIALAttr("id", para.ID)
	para.SetIALAttr("updated", "20261021120001")
	para.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("original text")})
	root.AppendChild(para)
	tree := &parse.Tree{ID: root.ID, Root: root, Box: "20261021115959-box0001", Path: "/nested/root.docj", HPath: "/Original title"}
	if err = indexdb.Transaction(ctx, owner, func(tx *stdsql.Tx) error { return indexTree(tx, tree, nil) }); err != nil {
		t.Fatal(err)
	}
	var rootROWID int64
	if err = owner.QueryRow("SELECT ROWID FROM blocks WHERE id=?", root.ID).Scan(&rootROWID); err != nil {
		t.Fatal(err)
	}
	para.FirstChild.Tokens = []byte("updated text")
	para.SetIALAttr("updated", "20261021120002")
	if err = indexdb.Transaction(ctx, owner, func(tx *stdsql.Tx) error { return upsertTree(tx, tree, nil) }); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		var count int
		var got int64
		if err = owner.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 2 {
			t.Fatal(table, count, err)
		}
		if err = owner.QueryRow("SELECT ROWID FROM "+table+" WHERE id=?", root.ID).Scan(&got); err != nil || got != rootROWID {
			t.Fatal("unchanged root rewritten", table, got, rootROWID, err)
		}
		var content string
		if err = owner.QueryRow("SELECT content FROM "+table+" WHERE id=?", para.ID).Scan(&content); err != nil || content != "updated text" {
			t.Fatal(table, content, err)
		}
		if err = other.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("other owner touched", table, count, err)
		}
	}
	para.Unlink()
	if err = indexdb.Transaction(ctx, owner, func(tx *stdsql.Tx) error { return upsertTree(tx, tree, nil) }); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		var count int
		if err = owner.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatal(table, count, err)
		}
	}
}
