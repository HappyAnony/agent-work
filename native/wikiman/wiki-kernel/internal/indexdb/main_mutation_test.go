//go:build fts5

package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
)

var originalMainTables = []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive", "spans", "assets", "refs", "file_annotation_refs", "attributes"}

func seedMainMutation(t *testing.T, db *sql.DB) {
	t.Helper()
	b := BlockRecord{ID: "root'quoted", RootID: "root'quoted", Box: "boxA", Path: "/old.docj", HPath: "/old", Content: "before", FContent: "before", Type: "d"}
	value := reflect.ValueOf(b)
	args := make([]interface{}, value.NumField())
	for i := range args {
		args[i] = value.Field(i).Interface()
	}
	for _, table := range originalMainTables {
		var err error
		switch table {
		case "blocks", "blocks_fts", "blocks_fts_case_insensitive":
			_, err = db.Exec("INSERT INTO "+table+" VALUES("+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+")", args...)
		case "assets":
			_, err = db.Exec("INSERT INTO assets(id,root_id,box,docpath,path,hash) VALUES('row',?,'boxA','/old.docj','assets/a.pdf',?)", b.ID, "hash'quoted")
		default:
			_, err = db.Exec("INSERT INTO "+table+"(id,root_id,box,path) VALUES('row',?,'boxA','/old.docj')", b.ID)
		}
		if err != nil {
			t.Fatal(table, err)
		}
	}
}

func TestOriginalMainNineMutationTableCoverageAndCaseRule(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		for _, action := range []string{"delete", "delete_id", "delete_ids", "rename", "rename_sub_tree", "delete_box", "delete_box_refs", "delete_refs", "delete_assets"} {
			t.Run(action+map[bool]string{true: "/sensitive", false: "/insensitive"}[sensitive], func(t *testing.T) {
				ctx := context.Background()
				bundle, err := OpenMemory(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer bundle.Close()
				db, _ := bundle.Database(Main)
				seedMainMutation(t, db)
				op := MainMutation{Action: action, ID: "root'quoted", Box: "boxA", Path: "/old.docj", HPath: "/new/中文 title", IAL: "new-ial", Updated: "20261021120000", IDs: []string{"absent", "root'quoted"}, Hashes: []string{"hash'quoted"}}
				if action == "rename_sub_tree" {
					op.Box = "moved-box"
					op.Path = "/moved.docj"
				}
				if err = Transaction(ctx, db, func(tx *sql.Tx) error { return ApplyMainMutation(ctx, tx, op, sensitive) }); err != nil {
					t.Fatal(err)
				}
				for _, table := range originalMainTables {
					var count int
					if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
						t.Fatal(err)
					}
					removed := action == "delete" || action == "delete_id" || action == "delete_ids" || action == "delete_box"
					if action == "delete_box_refs" || action == "delete_refs" {
						removed = table == "refs" || table == "file_annotation_refs"
					}
					if action == "delete_assets" {
						removed = table == "assets"
					}
					if sensitive && table == "blocks_fts_case_insensitive" {
						removed = false
					}
					expected := 1
					if removed {
						expected = 0
					}
					if count != expected {
						t.Fatalf("%s count %d want %d", table, count, expected)
					}
					if strings.HasPrefix(table, "blocks") && (action == "rename" || action == "rename_sub_tree") {
						var box, path, hpath, ial, content, fcontent, updated string
						if err = db.QueryRow("SELECT box,path,hpath,ial,content,fcontent,updated FROM "+table).Scan(&box, &path, &hpath, &ial, &content, &fcontent, &updated); err != nil {
							t.Fatal(err)
						}
						if sensitive && table == "blocks_fts_case_insensitive" {
							if content != "before" || hpath != "/old" {
								t.Fatal(table, content, hpath)
							}
							continue
						}
						if hpath != op.HPath || ial != op.IAL {
							t.Fatal(table, hpath, ial)
						}
						if action == "rename" && (content != "中文 title" || fcontent != content || updated != op.Updated || box != "boxA" || path != "/old.docj") {
							t.Fatal(table, box, path, content, fcontent, updated)
						}
						if action == "rename_sub_tree" && (box != "moved-box" || path != "/moved.docj" || content != "before") {
							t.Fatal(table, box, path, content)
						}
					}
				}
			})
		}
	}
}

func TestMainQueueFailedTailCopiesAndNeverReplaysCommittedPrefix(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	seedMainMutation(t, db)
	_, err = db.Exec("CREATE TABLE audit(content);CREATE TRIGGER audit_update AFTER UPDATE ON blocks BEGIN INSERT INTO audit VALUES(new.content); END;CREATE TRIGGER fail_delete BEFORE DELETE ON spans BEGIN SELECT RAISE(ABORT,'synthetic failure'); END;")
	if err != nil {
		t.Fatal(err)
	}
	var q ContentQueue
	if err = q.Update("root'quoted", "committed once"); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 513)
	for i := range ids {
		ids[i] = "absent"
	}
	ids[512] = "root'quoted"
	if err = q.Enqueue(MainMutation{Action: "delete_ids", IDs: ids}); err != nil {
		t.Fatal(err)
	}
	ids[512] = "caller mutated"
	var actions []string
	if err = q.FlushMain(ctx, db, false, func(action string) { actions = append(actions, action) }); err == nil {
		t.Fatal("failure swallowed")
	}
	if len(actions) != 1 || q.Pending() != 1 {
		t.Fatal(actions, q.Pending())
	}
	for _, table := range originalMainTables {
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 1 {
			t.Fatal("partial SQL deletion escaped rollback", table, count, err)
		}
	}
	if _, err = db.Exec("DROP TRIGGER fail_delete"); err != nil {
		t.Fatal(err)
	}
	if err = q.Close(ctx, db, false); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM audit").Scan(&count); err != nil || count != 1 {
		t.Fatal("committed prefix replayed", count, err)
	}
	for _, table := range originalMainTables {
		if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	if err = q.Enqueue(MainMutation{Action: "delete_box"}); !errors.Is(err, ErrContentQueueClosed) {
		t.Fatal(err)
	}
	if err = (&ContentQueue{}).Enqueue(MainMutation{Action: "upsert"}); !errors.Is(err, ErrMainMutation) {
		t.Fatal("unsupported tree work admitted", err)
	}
}

func TestMainQueueMutationRestoreKeepsLatestAndBatchFIFO(t *testing.T) {
	var q ContentQueue
	old := MainMutation{Action: "rename", ID: "A", HPath: "/old"}
	newer := MainMutation{Action: "rename", ID: "A", HPath: "/new"}
	if err := q.Enqueue(newer); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(MainMutation{Action: "delete_ids", IDs: []string{"B"}}); err != nil {
		t.Fatal(err)
	}
	q.restore([]contentUpdate{{mutation: &old}, {mutation: &MainMutation{Action: "delete_ids", IDs: []string{"C"}}}})
	if q.Pending() != 3 || q.pending[0].mutation.HPath != "/new" || q.pending[1].mutation.IDs[0] != "C" || q.pending[2].mutation.IDs[0] != "B" {
		t.Fatal(q.pending)
	}
}
