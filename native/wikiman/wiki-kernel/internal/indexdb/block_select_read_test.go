//go:build fts5

package indexdb

import (
	"context"
	"testing"
)

func TestSelectReadBlockRowsPreservesOrderLimitAndRejectsWrites(t *testing.T) {
	ctx := context.Background()
	b, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	db, _ := b.Database(Main)
	_, err = db.Exec("INSERT INTO blocks(id,type,content) VALUES('a','p','first'),('b','p','second'); UPDATE blocks SET parent_id='',root_id='',name='',markdown='',hash='',box='',path='',hpath='',alias='',memo='',tag='',fcontent='',length=0,subtype='',ial='',sort=0,created='',updated=''")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := SelectReadBlockRows(ctx, db, "SELECT * FROM blocks ORDER BY id", 2, 1)
	if err != nil || len(rows) != 1 || rows[0].ID != "b" {
		t.Fatal(rows, err)
	}
	for _, stmt := range []string{"DELETE FROM blocks", "SELECT * FROM blocks; DELETE FROM blocks", "SELECT * FROM missing_table"} {
		if _, err = SelectReadBlockRows(ctx, db, stmt, 1, 10); err == nil {
			t.Fatal(stmt)
		}
	}
	rows, err = SelectReadBlockRows(ctx, db, "WITH chosen AS (SELECT * FROM blocks) SELECT * FROM chosen ORDER BY id", 1, 10)
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = SelectReadBlockRows(canceled, db, "SELECT * FROM blocks", 1, 10); err == nil {
		t.Fatal("cancelled query")
	}
	if _, err = SelectReadBlockRows(nil, db, "SELECT * FROM blocks", 1, 10); err == nil {
		t.Fatal("nil context")
	}
}
