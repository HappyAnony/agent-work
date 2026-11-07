//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package indexdb

import (
	"context"
	"errors"
	"testing"
)

func TestRepairQueriesRejectTableInjectionAndPropagateScans(t *testing.T) {
	ctx := context.Background()
	b, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	db, _ := b.Database(Main)
	q := RepairQueries{DB: db}
	if _, e = q.DuplicatedRoots(ctx, "blocks`; DROP TABLE blocks; --"); !errors.Is(e, ErrRepairTable) {
		t.Fatal(e)
	}
	if _, e = q.DuplicatedRoots(ctx, "assets"); !errors.Is(e, ErrRepairTable) {
		t.Fatal(e)
	}
	if _, e = db.Exec("INSERT INTO blocks(id,root_id,type,updated,parent_id,hash,box,path,hpath,name,alias,memo,tag,content,fcontent,markdown,length,subtype,ial,sort,created) VALUES('id',NULL,'d','x','','','','','','','','','','','','',0,'','',0,''),('id',NULL,'d','x','','','','','','','','','','','','',0,'','',0,'')"); e != nil {
		t.Fatal(e)
	}
	if ids, e := q.DuplicatedRoots(ctx, "blocks"); e == nil || ids != nil {
		t.Fatal("bad scan swallowed", ids, e)
	}
	if rows, e := q.RootUpdated(ctx); e == nil || rows != nil {
		t.Fatal(rows, e)
	}
	if _, e = (RepairQueries{}).RootUpdated(ctx); !errors.Is(e, ErrNoDatabase) {
		t.Fatal(e)
	}
}
