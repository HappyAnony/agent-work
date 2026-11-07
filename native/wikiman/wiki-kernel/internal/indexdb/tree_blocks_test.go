//go:build fts5

package indexdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func treeBlockRows(n int) []BlockRecord {
	rows := make([]BlockRecord, n)
	for i := range rows {
		rows[i] = BlockRecord{ID: fmt.Sprintf("block'%04d", i), ParentID: "parent", RootID: "root'quoted", Hash: fmt.Sprint(i), Box: "box", Path: "/nested/root.docj", HPath: "/知识库", Name: "name", Alias: "alias", Memo: "memo", Tag: "#tag#", Content: "Searchable 内容", FContent: "first", Markdown: "**markdown**", Length: 17, Type: "p", SubType: "sub", IAL: "{: id=\"x\"}", Sort: 10, Created: "20261021120000", Updated: "20261021130000"}
	}
	return rows
}

func TestTreeBlocksOriginalColumnsBatchHashAndCaseRule(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		bundle, err := OpenMemory(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		db, _ := bundle.Database(Main)
		rows := treeBlockRows(513)
		var batches []BlockBatch
		err = Transaction(context.Background(), db, func(tx *sql.Tx) error {
			return InsertBlocks(context.Background(), tx, rows, sensitive, func(p BlockBatch) { batches = append(batches, p) })
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(batches) != 2 || batches[0].Count != 512 || batches[1].Count != 1 {
			t.Fatal(batches)
		}
		wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(rows[512].Hash+"blocksfts")))[:7]
		if batches[1].Hash != wantHash {
			t.Fatal(batches, wantHash)
		}
		for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
			var count int
			if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 513
			if sensitive && table == "blocks_fts_case_insensitive" {
				want = 0
			}
			if count != want {
				t.Fatal(table, count, want)
			}
			if want == 0 {
				continue
			}
			var got BlockRecord
			value := reflect.ValueOf(&got).Elem()
			args := make([]interface{}, value.NumField())
			for i := range args {
				args[i] = value.Field(i).Addr().Interface()
			}
			if err = db.QueryRow("SELECT * FROM "+table+" WHERE id=?", rows[512].ID).Scan(args...); err != nil {
				t.Fatal(err)
			}
			if got != rows[512] {
				t.Fatalf("%s %#v", table, got)
			}
		}
		bundle.Close()
	}
}

func TestTreeUpsertReadsWritingTransactionAndKeepsUnchangedROWID(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	rows := treeBlockRows(3)
	if err = Transaction(ctx, db, func(tx *sql.Tx) error { return InsertBlocks(ctx, tx, rows, false, nil) }); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err = db.QueryRow("SELECT ROWID FROM blocks WHERE id=?", rows[0].ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	// A write earlier in this transaction must be visible to the diff.
	next := []BlockRecord{rows[0], rows[1]}
	next[1].Hash, next[1].Content = "newhash", "new content"
	if err = Transaction(ctx, db, func(tx *sql.Tx) error {
		if _, e := tx.Exec("UPDATE blocks SET hash=? WHERE id=?", next[1].Hash, next[1].ID); e != nil {
			return e
		}
		changed, remove, e := SelectUpsertBlocks(ctx, tx, rows[0].RootID, next)
		if e != nil {
			return e
		}
		if len(changed) != 0 || !reflect.DeepEqual(remove, []string{rows[2].ID}) {
			t.Fatalf("transaction snapshot not used: %#v %v", changed, remove)
		}
		return DeleteIndexedBlocksByIDs(ctx, tx, remove, false)
	}); err != nil {
		t.Fatal(err)
	}
	var after int64
	if err = db.QueryRow("SELECT ROWID FROM blocks WHERE id=?", rows[0].ID).Scan(&after); err != nil || after != before {
		t.Fatal(after, before, err)
	}
	// Changed/new rows retain input order, deleted rows are removed separately.
	next[0].Hash = "changed"
	added := treeBlockRows(4)[3]
	next = append(next, added)
	if err = Transaction(ctx, db, func(tx *sql.Tx) error {
		changed, remove, e := SelectUpsertBlocks(ctx, tx, rows[0].RootID, next)
		if e != nil {
			return e
		}
		if len(changed) != 2 || changed[0] != next[0] || changed[1] != added {
			t.Fatal(changed)
		}
		if e = DeleteIndexedBlocksByIDs(ctx, tx, remove, false); e != nil {
			return e
		}
		return InsertBlocks(ctx, tx, changed, false, nil)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTreeBlocksLaterBatchFailureRollsBackAllMirrors(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	rows := treeBlockRows(513)
	if _, err = db.Exec("CREATE TRIGGER fail_last BEFORE INSERT ON blocks WHEN NEW.id = 'block''0512' BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	err = Transaction(ctx, db, func(tx *sql.Tx) error { return InsertBlocks(ctx, tx, rows, false, nil) })
	if err == nil {
		t.Fatal("missing batch failure")
	}
	for _, table := range mainBlockTables(false) {
		var count int
		if e := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); e != nil || count != 0 {
			t.Fatal(table, count, e)
		}
	}
	if _, err = db.Exec("DROP TRIGGER fail_last"); err != nil {
		t.Fatal(err)
	}
	if err = Transaction(ctx, db, func(tx *sql.Tx) error { return InsertBlocks(ctx, tx, rows, false, nil) }); err != nil {
		t.Fatal(err)
	}
	// Later mirror failure must also roll back the already deleted main rows.
	if _, err = db.Exec("CREATE TRIGGER fail_delete BEFORE DELETE ON blocks WHEN OLD.id = 'block''0512' BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	err = Transaction(ctx, db, func(tx *sql.Tx) error { return DeleteIndexedBlocksByIDs(ctx, tx, ids, false) })
	if err == nil {
		t.Fatal("missing delete failure")
	}
	for _, table := range mainBlockTables(false) {
		var count int
		if e := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); e != nil || count != 513 {
			t.Fatal(table, count, e)
		}
	}
	if _, err = db.Exec("DROP TRIGGER fail_delete"); err != nil {
		t.Fatal(err)
	}
	if err = Transaction(ctx, db, func(tx *sql.Tx) error { return DeleteIndexedBlocksByIDs(ctx, tx, ids, false) }); err != nil {
		t.Fatal(err)
	}
}

func TestTreeBlocksPhysicalROWIDAndOwnerIsolation(t *testing.T) {
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
	adb, _ := a.Database(Main)
	bdb, _ := b.Database(Main)
	for _, db := range []*sql.DB{adb, bdb} {
		if _, err = db.Exec("INSERT INTO blocks(ROWID,id,root_id,hash) VALUES(10,'target','root','h'); INSERT INTO blocks_fts(ROWID,id) VALUES(10,'physical-target'); INSERT INTO blocks_fts(ROWID,id) VALUES(20,'target'); INSERT INTO blocks_fts_case_insensitive(ROWID,id) VALUES(10,'keep-sensitive')"); err != nil {
			t.Fatal(err)
		}
	}
	if err = Transaction(ctx, adb, func(tx *sql.Tx) error { return DeleteIndexedBlocksByIDs(ctx, tx, []string{"target"}, true) }); err != nil {
		t.Fatal(err)
	}
	var id string
	if err = adb.QueryRow("SELECT id FROM blocks_fts").Scan(&id); err != nil || id != "target" {
		t.Fatal(id, err)
	}
	if err = adb.QueryRow("SELECT id FROM blocks_fts_case_insensitive").Scan(&id); err != nil || id != "keep-sensitive" {
		t.Fatal(id, err)
	}
	if err = bdb.QueryRow("SELECT id FROM blocks").Scan(&id); err != nil || id != "target" {
		t.Fatal(id, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = Transaction(ctx, bdb, func(tx *sql.Tx) error {
		_, _, e := SelectUpsertBlocks(cancelled, tx, "root", nil)
		return e
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = InsertBlocks(ctx, nil, nil, false, nil); !errors.Is(err, ErrNoDatabase) {
		t.Fatal(err)
	}
}

func TestTreeBlocksReadAndMirrorFailuresAbortInsteadOfEmptyDiff(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	if _, err = db.Exec("INSERT INTO blocks(id,root_id,hash) VALUES('keep','root',NULL)"); err != nil {
		t.Fatal(err)
	}
	if err = Transaction(ctx, db, func(tx *sql.Tx) error {
		_, _, e := SelectUpsertBlocks(ctx, tx, "root", nil)
		return e
	}); err == nil {
		t.Fatal("invalid hash was treated as an empty old index")
	}
	if _, err = db.Exec("DROP TABLE blocks_fts"); err != nil {
		t.Fatal(err)
	}
	if err = Transaction(ctx, db, func(tx *sql.Tx) error {
		return InsertBlocks(ctx, tx, treeBlockRows(1), false, nil)
	}); err == nil {
		t.Fatal("missing FTS mirror ignored")
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM blocks").Scan(&count); err != nil || count != 1 {
		t.Fatal("partial main insert survived", count, err)
	}
}
