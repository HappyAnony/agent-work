//go:build fts5

package sql

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"wiki-kernel/internal/indexdb"

	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

func TestOriginalConnectionTransactionsUseExplicitGateway(t *testing.T) {
	ctx := context.Background()
	bundle, err := indexdb.OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	previousMain, previousHistory, previousAssets := db, historyDB, assetContentDB
	defer func() { db, historyDB, assetContentDB = previousMain, previousHistory, previousAssets }()
	db, _ = bundle.Database(indexdb.Main)
	historyDB, _ = bundle.Database(indexdb.History)
	assetContentDB, _ = bundle.Database(indexdb.Assets)
	cases := []struct {
		begin     func() (*sql.Tx, error)
		commit    func(*sql.Tx) error
		statement string
	}{
		{beginTx, commitTx, `INSERT INTO blocks(id,content) VALUES('same-id','main')`},
		{beginHistoryTx, commitHistoryTx, `INSERT INTO histories_fts_case_insensitive(id,title) VALUES('same-id','history')`},
		{beginAssetContentTx, commitAssetContentTx, `INSERT INTO asset_contents_fts_case_insensitive(id,name) VALUES('same-id','assets')`},
	}
	for _, item := range cases {
		tx, err := item.begin()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(item.statement); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = item.commit(tx); err != nil {
			t.Fatal(err)
		}
	}
	var got string
	if err = queryRow(`SELECT content FROM blocks WHERE id='same-id'`).Scan(&got); err != nil || got != "main" {
		t.Fatal(got, err)
	}
	rows, err := query(`SELECT id FROM blocks`)
	if err != nil {
		t.Fatal(err)
	}
	rows.Close()
}

func TestLegacyDerivedDatabaseFirstOpenAndReopen(t *testing.T) {
	root := t.TempDir()
	oldData, oldDB, oldHistory, oldAsset, oldTree := util.DataDir, util.DBPath, util.HistoryDBPath, util.AssetContentDBPath, util.BlockTreeDBPath
	defer func() {
		util.DataDir, util.DBPath, util.HistoryDBPath, util.AssetContentDBPath, util.BlockTreeDBPath = oldData, oldDB, oldHistory, oldAsset, oldTree
	}()
	util.DataDir = filepath.Join(root, "data")
	if err := os.Mkdir(util.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	util.DBPath = filepath.Join(root, "main.db")
	util.HistoryDBPath = filepath.Join(root, "history.db")
	util.AssetContentDBPath = filepath.Join(root, "assets.db")
	util.BlockTreeDBPath = filepath.Join(root, "tree.db")
	if err := InitDatabase(false); err != nil {
		t.Fatal(err)
	}
	InitHistoryDatabase(false)
	InitAssetContentDatabase(false)
	defer CloseDatabase()
	if _, err := db.Exec(`INSERT INTO blocks(id,content) VALUES('same-id','original')`); err != nil {
		t.Fatal(err)
	}
	if _, err := historyDB.Exec(`INSERT INTO histories_fts_case_insensitive(id,title) VALUES('same-id','history')`); err != nil {
		t.Fatal("first history schema missing", err)
	}
	if _, err := assetContentDB.Exec(`INSERT INTO asset_contents_fts_case_insensitive(id,name) VALUES('same-id','asset')`); err != nil {
		t.Fatal("first assets schema missing", err)
	}
	if got := treenode.CountBlocks(); got != 0 {
		t.Fatal("first blocktree schema", got)
	}
	InitHistoryDatabase(false)
	InitAssetContentDatabase(false)
	if err := InitDatabase(false); err != nil {
		t.Fatal(err)
	}
	for _, query := range []struct {
		q        string
		expected string
	}{{`SELECT content FROM blocks WHERE id='same-id'`, "original"}} {
		var got string
		if err := db.QueryRow(query.q).Scan(&got); err != nil || got != query.expected {
			t.Fatal("reopen lost index", got, err)
		}
	}
	var got string
	if err := historyDB.QueryRow(`SELECT title FROM histories_fts_case_insensitive WHERE id='same-id'`).Scan(&got); err != nil || got != "history" {
		t.Fatal(got, err)
	}
	if err := assetContentDB.QueryRow(`SELECT name FROM asset_contents_fts_case_insensitive WHERE id='same-id'`).Scan(&got); err != nil || got != "asset" {
		t.Fatal(got, err)
	}
}
