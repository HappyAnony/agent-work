//go:build fts5

package indexdb

import (
	"context"
	"errors"
	"testing"
)

func TestOriginalSchemasAndIsolation(t *testing.T) {
	ctx := context.Background()
	a, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for _, kind := range []Kind{Main, History, Assets, BlockTree} {
		x, _ := a.Database(kind)
		y, _ := b.Database(kind)
		if x == y {
			t.Fatal("shared handle")
		}
		if _, err = x.ExecContext(ctx, `CREATE TABLE isolation(id TEXT);INSERT INTO isolation VALUES('same-id')`); err != nil {
			t.Fatal(kind, err)
		}
		if _, err = y.ExecContext(ctx, `SELECT * FROM isolation`); err == nil {
			t.Fatal("shared schema")
		}
	}
	main, _ := a.Database(Main)
	if _, err = main.ExecContext(ctx, `INSERT INTO blocks(id,parent_id,root_id,box,path,content) VALUES('same-id','parent','root','box','/root.docj','original')`); err != nil {
		t.Fatal(err)
	}
	if _, err = main.ExecContext(ctx, `INSERT INTO blocks_fts(id,content) VALUES('same-id','原生索引 wiki original')`); err != nil {
		t.Fatal(err)
	}
	var found string
	if err = main.QueryRowContext(ctx, `SELECT id FROM blocks_fts WHERE blocks_fts MATCH 'wiki'`).Scan(&found); err != nil || found != "same-id" {
		t.Fatal(found, err)
	}
	if err = main.QueryRowContext(ctx, `SELECT value FROM stat WHERE key='siyuan_database_ver'`).Scan(&found); err != nil || found != SchemaVersion {
		t.Fatal(found, err)
	}
	other, _ := b.Database(Main)
	var count int
	if err = other.QueryRow(`SELECT count(*) FROM blocks`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	// Cancellation may discard the work connection, not the schema keeper.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_ = main.QueryRowContext(cancelled, `SELECT 1`).Scan(&count)
	if err = main.QueryRow(`SELECT count(*) FROM blocks`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Database(Main); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err = main.Ping(); err == nil {
		t.Fatal("retained closed handle remained usable")
	}
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestResetFailureIsAtomic(t *testing.T) {
	ctx := context.Background()
	b, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	db, _ := b.Database(Main)
	if _, err = db.Exec(`INSERT INTO stat VALUES('keep','value');CREATE VIEW spans AS SELECT 1`); err == nil {
		t.Fatal("spans table unexpectedly missing")
	}
	// A view with a table's name makes the later DROP TABLE fail after earlier DDL.
	if _, err = db.Exec(`DROP TABLE spans;CREATE VIEW spans AS SELECT 1`); err != nil {
		t.Fatal(err)
	}
	if err = Reset(ctx, db, Main, SchemaVersion); err == nil {
		t.Fatal("reset unexpectedly succeeded")
	}
	var got string
	if err = db.QueryRow(`SELECT value FROM stat WHERE key='keep'`).Scan(&got); err != nil || got != "value" {
		t.Fatal("partial DDL published", got, err)
	}
}
