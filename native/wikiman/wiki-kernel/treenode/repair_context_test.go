//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package treenode

import (
	"context"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestRepairRegistryRetainsHandleAndReportsErrors(t *testing.T) {
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
	db, _ = a.Database(indexdb.BlockTree)
	if _, e = db.Exec("INSERT INTO blocktrees(id,root_id,box_id,path,type,parent_id,hpath,updated) VALUES('id','id','box','/a.docj','d','','','')"); e != nil {
		t.Fatal(e)
	}
	q := CaptureLegacyRepairRegistry()
	db, _ = b.Database(indexdb.BlockTree)
	if _, e = db.Exec("INSERT INTO blocktrees(id,root_id,box_id,path,type,parent_id,hpath,updated) VALUES('id','id','box','/other.docj','d','','','')"); e != nil {
		t.Fatal(e)
	}
	_, missing, e := q.PathDifference(ctx, "box", []string{"/a.docj"})
	if e != nil || len(missing) != 0 {
		t.Fatal(missing, e)
	}
	if _, e = q.Remove(ctx, indexdb.BlockTreeFilter{Kind: "root", Value: "id"}); e != nil {
		t.Fatal(e)
	}
	n, e := (indexdb.BlockTrees{DB: db}).Count(ctx, indexdb.BlockTreeFilter{Kind: "all"})
	if e != nil || n != 1 {
		t.Fatal("cross-owner deletion", n, e)
	}
	q.DB.Close()
	if _, _, e = q.PathDifference(ctx, "box", nil); e == nil {
		t.Fatal("closed registry became an empty scan")
	}
}
