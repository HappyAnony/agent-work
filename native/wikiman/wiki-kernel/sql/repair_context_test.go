//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package sql

import (
	"context"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestRepairQueriesRetainCapturedDatabaseAndIgnoreStaleCache(t *testing.T) {
	ctx := context.Background()
	a, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	old := db
	defer func() { db = old }()
	db, _ = a.Database(indexdb.Main)
	if _, e = db.Exec("INSERT INTO blocks(id,root_id,type,updated,content,parent_id,hash,box,path,hpath,name,alias,memo,tag,fcontent,markdown,length,subtype,ial,sort,created) VALUES('id','id','d','first','first','','','','','','','','','','','',0,'','',0,'')"); e != nil {
		t.Fatal(e)
	}
	q := CaptureLegacyRepairQueries()
	db, _ = b.Database(indexdb.Main)
	if _, e = db.Exec("INSERT INTO blocks(id,root_id,type,updated,content,parent_id,hash,box,path,hpath,name,alias,memo,tag,fcontent,markdown,length,subtype,ial,sort,created) VALUES('id','id','d','other','other','','','','','','','','','','','',0,'','',0,'')"); e != nil {
		t.Fatal(e)
	}
	rows, e := q.Blocks(ctx, []string{"id"})
	if e != nil || len(rows) != 1 || rows[0].Content != "first" {
		t.Fatal(rows, e)
	}
	if _, e = q.DB.Exec("UPDATE blocks SET content='changed',updated='changed'"); e != nil {
		t.Fatal(e)
	}
	rows, e = q.Blocks(ctx, []string{"id"})
	if e != nil || rows[0].Content != "changed" {
		t.Fatal("audit reused cached row", rows, e)
	}
	updates, e := q.RootUpdated(ctx)
	if e != nil || updates["id"] != "changed" {
		t.Fatal(updates, e)
	}
}
