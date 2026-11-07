//go:build fts5

package indexdb

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"testing"
	"wiki-kernel/conf"
)

func TestReplacementSearchAllBeyondPageLimitRetainsOrdinaryBounds(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, err := bundle.Database(Main)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]BlockRecord, 10005)
	for i := range rows {
		rows[i] = BlockRecord{ID: fmt.Sprintf("row-%05d", i), RootID: "root", Box: "owner", Type: "p", Content: "needle value", Sort: i}
	}
	if err = Transaction(ctx, db, func(tx *sql.Tx) error { return InsertBlocks(ctx, tx, rows, false, nil) }); err != nil {
		t.Fatal(err)
	}
	settings := *conf.NewSearch()
	request := BlockSearchRequest{Query: "needle", Page: 2, PageSize: 2}
	all, err := SearchAllBlocks(ctx, db, settings, request)
	if err != nil || len(all.Blocks) != len(rows) || all.MatchedBlocks != len(rows) || all.PageCount != 1 {
		t.Fatal("replace-all truncated or count overflow", len(all.Blocks), all.MatchedBlocks, all.PageCount, err)
	}
	normal, err := SearchBlocks(ctx, db, settings, request)
	if err != nil || len(normal.Blocks) != 2 || normal.MatchedBlocks != len(rows) {
		t.Fatal(normal, err)
	}
	request.PageSize = math.MaxInt
	if _, err = SearchBlocks(ctx, db, settings, request); err == nil {
		t.Fatal("ordinary search lost page-size gate")
	}
	request.Query = "needle"
	request.Ignore = []string{"box='elsewhere'"}
	all, err = SearchAllBlocks(ctx, db, settings, request)
	if err != nil || len(all.Blocks) != 0 {
		t.Fatal("owner ignore policy bypassed", err)
	}
	request.Ignore = nil
	request.Method = 2
	request.Query = "DELETE FROM blocks"
	if _, err = SearchAllBlocks(ctx, db, settings, request); err == nil {
		t.Fatal("all-query bypassed read-only SQL validation")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = SearchAllBlocks(cancelled, db, settings, BlockSearchRequest{Query: "needle"}); err == nil {
		t.Fatal("cancelled all-query succeeded")
	}
}
