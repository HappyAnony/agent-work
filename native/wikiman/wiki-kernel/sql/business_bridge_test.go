//go:build fts5

package sql

import (
	"context"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalUpperQueryAndHistoryQueueBridge(t *testing.T) {
	ctx := context.Background()
	bundle, err := indexdb.OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	oldDB, oldHistory := db, historyDB
	defer func() { db, historyDB = oldDB, oldHistory }()
	db, _ = bundle.Database(indexdb.Main)
	historyDB, _ = bundle.Database(indexdb.History)
	for range 2 {
		if _, err = db.Exec("INSERT INTO refs(def_block_id,def_block_root_id,block_id) VALUES('same-id','root','ref')"); err != nil {
			t.Fatal(err)
		}
	}
	if QueryRefCount([]string{"same-id"})["same-id"] != 2 || QueryRootChildrenRefCount("root")["same-id"] != 2 || QueryRootBlockRefCount()["root"] != 1 || len(GetRefDuplicatedDefRootIDs()) != 1 {
		t.Fatal("original wrappers diverged")
	}
	h := &History{ID: "same-id", Title: "original", Created: "20261021"}
	IndexHistoriesQueue([]*History{h})
	h.Title = "mutated"
	FlushHistoryQueue()
	var title string
	if err = historyDB.QueryRow("SELECT title FROM histories_fts_case_insensitive").Scan(&title); err != nil || title != "original" {
		t.Fatal(title, err)
	}
	DeleteOutdatedHistories(20261022)
	FlushHistoryQueue()
	var count int
	if err = historyDB.QueryRow("SELECT COUNT(*) FROM histories_fts_case_insensitive").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}
