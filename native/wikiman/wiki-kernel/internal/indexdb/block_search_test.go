//go:build fts5

package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"wiki-kernel/conf"
)

func TestOriginalSearchModesCountsPagingFiltersAndReadOnlySQL(t *testing.T) {
	ctx := context.Background()
	bundle, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	rows := []BlockRecord{{ID: "root", RootID: "root", Box: "box'1", Path: "/root.docj", HPath: "/doc", Type: "d", Content: "alpha", Updated: "20261022"}, {ID: "one", RootID: "root", ParentID: "root", Box: "box'1", Path: "/root.docj", HPath: "/doc", Type: "p", Content: "beta alpha", Name: "named", Updated: "20261021"}, {ID: "two", RootID: "other", Box: "box2", Path: "/other.docj", Type: "p", Content: "gamma", Updated: "20261020"}}
	if e = Transaction(ctx, db, func(tx *sql.Tx) error { return InsertBlocks(ctx, tx, rows, false, nil) }); e != nil {
		t.Fatal(e)
	}
	settings := *conf.NewSearch()
	request := BlockSearchRequest{Query: "alpha", Page: 1, PageSize: 1}
	r, e := SearchBlocks(ctx, db, settings, request)
	if e != nil || len(r.Blocks) != 1 || r.MatchedBlocks != 2 || r.MatchedRoots != 1 || r.PageCount != 2 || !strings.Contains(r.Blocks[0].Content, "__@mark__") {
		t.Fatal(r, e)
	}
	request.Page = 2
	r, e = SearchBlocks(ctx, db, settings, request)
	if e != nil || len(r.Blocks) != 1 {
		t.Fatal(r, e)
	}
	request.Page = 1
	request.PageSize = 32
	request.Method = 1
	request.Query = "alpha OR gamma"
	r, e = SearchBlocks(ctx, db, settings, request)
	if e != nil || r.MatchedBlocks != 3 || r.DocumentMode {
		t.Fatal(r, e)
	}
	request.Method = 3
	request.Query = "alp.*"
	r, e = SearchBlocks(ctx, db, settings, request)
	if e != nil || r.MatchedBlocks != 2 || len(r.Blocks) != 2 {
		t.Fatal(r, e)
	}
	request.Query = "["
	if _, e = SearchBlocks(ctx, db, settings, request); e == nil {
		t.Fatal("invalid regexp succeeded")
	}
	request.Method = 0
	request.Query = "alpha beta"
	r, e = SearchBlocks(ctx, db, settings, request)
	if e != nil || !r.DocumentMode || r.MatchedRoots != 1 || r.MatchedBlocks != 1 {
		t.Fatal(r, e)
	}
	request.Query = "alpha"
	request.Boxes = []string{"box'1"}
	r, e = SearchBlocks(ctx, db, settings, request)
	if e != nil || r.MatchedBlocks != 2 {
		t.Fatal(r, e)
	}
	request.Method = 2
	request.Query = "SELECT * FROM blocks WHERE content LIKE '%alpha%'"
	request.PageSize = 1
	r, e = SearchBlocks(ctx, db, settings, request)
	if e != nil || r.MatchedBlocks != 2 || len(r.Blocks) != 1 {
		t.Fatal(r, e)
	}
	for _, query := range []string{"DELETE FROM blocks", "PRAGMA query_only=off", "SELECT * FROM blocks; DELETE FROM blocks", "SELECT * FROM blocks; SELECT * FROM refs"} {
		request.Query = query
		if _, e = SearchBlocks(ctx, db, settings, request); e == nil {
			t.Fatal("unsafe SQL accepted", query)
		}
	}
	request.Method = 0
	request.Query = "alpha"
	request.Ignore = []string{"1=1; DELETE FROM blocks; --"}
	if _, e = SearchBlocks(ctx, db, settings, request); e == nil {
		t.Fatal("source ignore policy allowed multiple statements")
	}
	request.Ignore = nil
	request.Page = 0
	if _, e = SearchBlocks(ctx, db, settings, request); e == nil {
		t.Fatal("invalid page")
	}
	request.Page = 1
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = SearchBlocks(canceled, db, settings, request); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	var count int
	if e = db.QueryRow("SELECT COUNT(*) FROM blocks").Scan(&count); e != nil || count != 3 {
		t.Fatal("read changed DB", count, e)
	}
}

func TestReadSQLQuotedIdentifiersAndComments(t *testing.T) {
	for _, stmt := range []string{
		"SELECT `id` FROM `blocks` WHERE content='literal ` value'",
		"WITH c AS (SELECT * FROM `blocks`) SELECT * FROM c",
		"SELECT * FROM blocks UNION SELECT * FROM blocks",
		"SELECT 'literal; DELETE FROM blocks' /* ` */ FROM blocks -- `\n",
	} {
		if e := ValidateReadSQL(stmt); e != nil {
			t.Fatal(stmt, e)
		}
	}
	for _, stmt := range []string{
		"SELECT `id` FROM blocks; DELETE FROM blocks",
		"WITH c AS (SELECT * FROM blocks) DELETE FROM blocks",
		"SELECT * FROM blocks /* comment */; PRAGMA query_only=off",
	} {
		if e := ValidateReadSQL(stmt); e == nil {
			t.Fatal("unsafe SQL accepted", stmt)
		}
	}
}
