//go:build fts5

package sql

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalAssetAndMainContentBusinessBridge(t *testing.T) {
	ctx := context.Background()
	bundle, err := indexdb.OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	oldMain, oldAsset, oldCase := db, assetContentDB, caseSensitive
	defer func() { db, assetContentDB, caseSensitive = oldMain, oldAsset, oldCase; ClearQueue() }()
	db, _ = bundle.Database(indexdb.Main)
	assetContentDB, _ = bundle.Database(indexdb.Assets)
	caseSensitive = false
	ClearQueue()
	asset := &AssetContent{ID: "same-id", Name: "original", Path: "assets/a.pdf", Content: "text"}
	IndexAssetContentsQueue([]*AssetContent{asset, asset, asset})
	asset.Name = "mutated"
	FlushAssetContentQueue()
	rows := SelectAssetContentsRawStmtNoParse("SELECT * FROM asset_contents_fts_case_insensitive", 1)
	if len(rows) != 2 || rows[0].Name != "original" {
		t.Fatal("original row/limit semantics changed", rows)
	}
	DeleteAssetContentsByPathQueue(asset.Path)
	FlushAssetContentQueue()
	if rows := SelectAssetContentsRawStmtNoParse("SELECT * FROM asset_contents_fts_case_insensitive LIMIT 10", 1); len(rows) != 0 {
		t.Fatal(rows)
	}
	block := Block{ID: "same-id", Type: "query_embed", Markdown: "{{SELECT * FROM blocks}}"}
	value := reflect.ValueOf(block)
	args := make([]interface{}, value.NumField())
	for i := range args {
		args[i] = value.Field(i).Interface()
	}
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		if _, err = db.Exec("INSERT INTO "+table+" VALUES("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args...); err != nil {
			t.Fatal(err)
		}
	}
	if blocks := QueryEmptyContentEmbedBlocks(); len(blocks) != 1 || blocks[0].Markdown != block.Markdown {
		t.Fatal(blocks)
	}
	block.Content = "first"
	UpdateBlockContentQueue(&block)
	block.Content = "last"
	UpdateBlockContentQueue(&block)
	block.Content = "mutated after admission"
	FlushQueue()
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
		var text string
		if err = db.QueryRow("SELECT content FROM " + table + " WHERE id='same-id'").Scan(&text); err != nil || text != "last" {
			t.Fatal(text, err)
		}
	}
}
