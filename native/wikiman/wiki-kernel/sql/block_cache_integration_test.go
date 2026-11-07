//go:build fts5

package sql

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalGetBlocksUsesSharedCacheAndActualDatabase(t *testing.T) {
	ctx := context.Background()
	bundle, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	owner, _ := bundle.Database(indexdb.Main)
	row := Block{ID: "quoted'id", Content: "actual SQL"}
	v := reflect.ValueOf(row)
	args := make([]interface{}, v.NumField())
	for i := range args {
		args[i] = v.Field(i).Interface()
	}
	if _, e = owner.Exec("INSERT INTO blocks VALUES("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args...); e != nil {
		t.Fatal(e)
	}
	oldDB := db
	db = owner
	defer func() { db = oldDB }()
	if e = OpenLegacyCaches(); e != nil {
		t.Fatal(e)
	}
	defer CloseLegacyCaches()
	enableCache()
	got := GetBlocks([]string{"missing", row.ID, row.ID})
	if len(got) != 3 || got[0] != nil || got[1].Content != row.Content || got[1] == got[2] {
		t.Fatal(got)
	}
	got[1].Content = "borrowed"
	if GetBlock(row.ID).Content != row.Content {
		t.Fatal("borrowed SQL cache")
	}
	if _, e = owner.Exec("UPDATE blocks SET content='updated'"); e != nil {
		t.Fatal(e)
	}
	if GetBlock(row.ID).Content != row.Content {
		t.Fatal("cache unexpectedly bypassed")
	}
	ClearCache()
	if GetBlock(row.ID).Content != "updated" {
		t.Fatal("clear retained stale row")
	}
	disableCache()
	if _, e = owner.Exec("UPDATE blocks SET content='disabled'"); e != nil {
		t.Fatal(e)
	}
	if GetBlock(row.ID).Content != "disabled" {
		t.Fatal("disabled cache used")
	}
}
