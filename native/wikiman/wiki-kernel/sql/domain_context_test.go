//go:build fts5

package sql

import (
	"context"
	stdsql "database/sql"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalSearchAndTagFacadesUseContextAlgorithms(t *testing.T) {
	ctx := context.Background()
	bundle, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	previous := db
	db, _ = bundle.Database(indexdb.Main)
	defer func() { db = previous }()
	if e = indexdb.Transaction(ctx, db, func(tx *stdsql.Tx) error {
		if e := indexdb.InsertBlocks(ctx, tx, []indexdb.BlockRecord{{ID: "root", RootID: "root", Type: "d", Content: "alpha doc"}, {ID: "child", ParentID: "root", RootID: "root", Type: "p", Content: "beta alpha"}}, false, nil); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, "INSERT INTO spans(id,block_id,root_id,box,path,content,markdown,type,ial) VALUES(?,?,?,?,?,?,?,?,?)", "tag1", "child", "root", "box", "/doc.docj", "AGI's 中文", "#AGI's 中文#", "tag", "")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	for _, method := range []int{0, 1, 2, 3} {
		query := "alpha"
		if method == 2 {
			query = "SELECT * FROM blocks WHERE content LIKE '%alpha%'"
		}
		request := indexdb.BlockSearchRequest{Query: query, Method: method, Page: 1, PageSize: 32}
		got, e := SearchBlocks(ctx, *conf.NewSearch(), request)
		if e != nil {
			t.Fatal(e)
		}
		want, e := indexdb.SearchBlocks(ctx, db, *conf.NewSearch(), request)
		if e != nil || !reflect.DeepEqual(got, want) || got.MatchedBlocks != 2 {
			t.Fatal(method, got, want, e)
		}
	}
	if rows := QueryTagSpansByLabel("AGI's"); len(rows) != 1 || rows[0].BlockID != "child" {
		t.Fatal(rows)
	}
	if rows := QueryTagSpansByKeyword("AGI's 中文", 32); len(rows) != 1 {
		t.Fatal(rows)
	}
	if rows := QueryTagSpans("/doc.docj"); len(rows) != 1 {
		t.Fatal(rows)
	}
	got, e := SearchTagLabels(ctx, "AGI's 中文", 32, false)
	if e != nil || len(got) != 1 || !strings.Contains(got[0], "<mark>") {
		t.Fatal(got, e)
	}
	raw := SelectBlocksRawStmt("SELECT * FROM blocks ORDER BY id", 1, 32)
	if len(raw) != 2 {
		t.Fatal(raw)
	}
}
