//go:build fts5

// SPDX-License-Identifier: AGPL-3.0-or-later
package indexrepair

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func fixture(t *testing.T) (*sql.DB, indexdb.BlockTrees) {
	t.Helper()
	b, e := indexdb.OpenMemory(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { b.Close() })
	db, _ := b.Database(indexdb.Main)
	tree, _ := b.Database(indexdb.BlockTree)
	return db, indexdb.BlockTrees{DB: tree}
}
func exec(t *testing.T, db *sql.DB, s string, args ...interface{}) {
	t.Helper()
	if _, e := db.Exec(s, args...); e != nil {
		t.Fatal(e)
	}
}
func TestRepairPlanOriginalToleranceMigrationRemovalAndIsolation(t *testing.T) {
	ctx := context.Background()
	db, trees := fixture(t)
	other, otherTrees := fixture(t)
	for _, v := range []struct{ id, updated string }{{"missing", "20261028100000"}, {"migration", ""}, {"late", "20261028103000"}, {"edge", "20261028103000"}, {"recent", "20261028103000"}} {
		exec(t, trees.DB, "INSERT INTO blocktrees(id,root_id,type,updated,parent_id,box_id,path,hpath) VALUES(?,?,'d',?,'','','','')", v.id, v.id, v.updated)
	}
	for _, v := range []struct{ id, updated string }{{"migration", "20261028100000"}, {"late", "20261028101959"}, {"edge", "20261028102000"}, {"recent", "20261028102100"}, {"orphan", "20261028103000"}} {
		exec(t, db, "INSERT INTO blocks(id,root_id,type,updated,parent_id,hash,box,path,hpath,name,alias,memo,tag,content,fcontent,markdown,length,subtype,ial,sort,created) VALUES(?,?,'d',?,'','','','','','','','','','','','',0,'','',0,'')", v.id, v.id, v.updated)
	}
	p, e := Inspect(ctx, indexdb.RepairQueries{DB: db}, trees, false)
	if e != nil || !reflect.DeepEqual(p.ReindexRoots, []string{"late", "migration", "missing"}) || !reflect.DeepEqual(p.RemoveRoots, []string{"orphan"}) {
		t.Fatal(p, e)
	}
	empty, e := Inspect(ctx, indexdb.RepairQueries{DB: other}, otherTrees, false)
	if e != nil || !reflect.DeepEqual(empty, Plan{}) {
		t.Fatal("owner isolation", empty, e)
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM blocks").Scan(&n)
	if n != 5 {
		t.Fatal("inspection mutated indexes", n)
	}
	p.ReindexRoots[0] = "changed"
	again, e := Inspect(ctx, indexdb.RepairQueries{DB: db}, trees, false)
	if e != nil || again.ReindexRoots[0] != "late" {
		t.Fatal("aliased plan", again, e)
	}
}
func TestRepairPlanOriginalDuplicateTableFallbackAndRefs(t *testing.T) {
	db, trees := fixture(t)
	ctx := context.Background()
	exec(t, db, "INSERT INTO blocks(id,root_id,type,updated,parent_id,hash,box,path,hpath,name,alias,memo,tag,content,fcontent,markdown,length,subtype,ial,sort,created) VALUES('root','root','d','20261028100000','','','','','','','','','','','','',0,'','',0,'')")
	for i := 0; i < 2; i++ {
		exec(t, db, "INSERT INTO blocks_fts_case_insensitive(id,root_id) VALUES('dup','root')")
		exec(t, db, "INSERT INTO refs(id,def_block_id,def_block_root_id,block_id) VALUES('ref','def','root','using')")
	}
	p, e := Inspect(ctx, indexdb.RepairQueries{DB: db}, trees, false)
	if e != nil || !reflect.DeepEqual(p.DuplicateRoots, []string{"root"}) || !reflect.DeepEqual(p.DuplicateReferenceRoots, []string{"root"}) {
		t.Fatal(p, e)
	}
	p, e = Inspect(ctx, indexdb.RepairQueries{DB: db}, trees, true)
	if e != nil || len(p.DuplicateRoots) != 0 {
		t.Fatal("case-sensitive fallback", p, e)
	}
	exec(t, db, "INSERT INTO blocks_fts(id,root_id) VALUES('ftsdup','root'),('ftsdup','root')")
	p, e = Inspect(ctx, indexdb.RepairQueries{DB: db}, trees, true)
	if e != nil || !reflect.DeepEqual(p.DuplicateRoots, []string{"root"}) {
		t.Fatal("fts fallback", p, e)
	}
	exec(t, db, "INSERT INTO blocks(id,root_id,type,updated,parent_id,hash,box,path,hpath,name,alias,memo,tag,content,fcontent,markdown,length,subtype,ial,sort,created) VALUES('root','root','d','20261028100000','','','','','','','','','','','','',0,'','',0,'')")
	exec(t, db, "DROP TABLE blocks_fts")
	p, e = Inspect(ctx, indexdb.RepairQueries{DB: db}, trees, true)
	if e != nil || len(p.DuplicateRoots) != 1 {
		t.Fatal("nonempty first table must skip fallback", p, e)
	}
}
func TestRepairPlanReadFailuresCancelAndClosedNeverReturnPartialCandidates(t *testing.T) {
	for _, fault := range []string{"cancel", "missing-fts", "missing-tree", "invalid-updated", "closed"} {
		t.Run(fault, func(t *testing.T) {
			db, trees := fixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			exec(t, db, "INSERT INTO blocks(id,root_id,type,updated,parent_id,hash,box,path,hpath,name,alias,memo,tag,content,fcontent,markdown,length,subtype,ial,sort,created) VALUES('orphan','orphan','d','20261028100000','','','','','','','','','','','','',0,'','',0,'')")
			switch fault {
			case "cancel":
				cancel()
			case "missing-fts":
				exec(t, db, "DROP TABLE blocks_fts")
			case "missing-tree":
				exec(t, trees.DB, "DROP TABLE blocktrees")
			case "invalid-updated":
				exec(t, db, "UPDATE blocks SET updated=NULL")
			case "closed":
				db.Close()
			}
			p, e := Inspect(ctx, indexdb.RepairQueries{DB: db}, trees, false)
			if e == nil || !reflect.DeepEqual(p, Plan{}) {
				t.Fatal("failed read became a partial actionable plan", p, e)
			}
			if fault == "cancel" && !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
		})
	}
}
