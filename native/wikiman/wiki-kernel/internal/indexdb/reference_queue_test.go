//go:build fts5

package indexdb

import (
	"context"
	"errors"
	"testing"
)

func TestReferenceQueueBatchesCopiesAndRollsBackReplacement(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	if _, err = db.Exec("INSERT INTO refs(id,box,path,content) VALUES('old','box','/a.docj','old');INSERT INTO refs(id,box,path,content) VALUES('other','box','/other.docj','keep');CREATE TRIGGER fail_annotation BEFORE INSERT ON file_annotation_refs BEGIN SELECT RAISE(ABORT,'synthetic failure'); END"); err != nil {
		t.Fatal(err)
	}
	refs := make([]RefRecord, 513)
	for i := range refs {
		refs[i] = RefRecord{ID: "same", RootID: "doc", Box: "box", Path: "/a.docj", Content: "copied"}
	}
	annotations := []AnnotationRecord{{ID: "annotation", Box: "box", Path: "/a.docj"}}
	var q ContentQueue
	if err = q.Enqueue(MainMutation{Action: "update_refs", ID: "doc", Box: "box", Path: "/a.docj", Refs: refs, Annotations: annotations}); err != nil {
		t.Fatal(err)
	}
	refs[0].Content = "mutated"
	annotations[0].Path = "mutated"
	if err = q.Flush(ctx, db, false); err == nil {
		t.Fatal("replacement error swallowed")
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM refs WHERE content='old'").Scan(&count); err != nil || count != 1 || q.Pending() != 1 {
		t.Fatal("old refs lost before replacement committed", count, q.Pending(), err)
	}
	if _, err = db.Exec("DROP TRIGGER fail_annotation"); err != nil {
		t.Fatal(err)
	}
	if err = q.Close(ctx, db, false); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM refs WHERE content='copied'").Scan(&count); err != nil || count != 513 {
		t.Fatal(count, err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM refs WHERE content='keep'").Scan(&count); err != nil || count != 1 {
		t.Fatal("unrelated document deleted", count, err)
	}
	if err = db.QueryRow("SELECT COUNT(*) FROM file_annotation_refs WHERE path='/a.docj'").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestIndexNodeKeepsOriginalCaseRuleAndDistinctFIFOAction(t *testing.T) {
	for _, sensitive := range []bool{false, true} {
		ctx := context.Background()
		bundle, err := OpenMemory(ctx)
		if err != nil {
			t.Fatal(err)
		}
		db, _ := bundle.Database(Main)
		seedMainMutation(t, db)
		var q ContentQueue
		if err = q.Update("root'quoted", "first"); err != nil {
			t.Fatal(err)
		}
		if err = q.Enqueue(MainMutation{Action: "index_node", ID: "root'quoted", Content: "node content"}); err != nil {
			t.Fatal(err)
		}
		if err = q.Update("root'quoted", "last content update"); err != nil {
			t.Fatal(err)
		}
		if q.Pending() != 2 {
			t.Fatal("actions incorrectly coalesced together", q.Pending())
		}
		if err = q.Flush(ctx, nil, sensitive); !errors.Is(err, ErrNoDatabase) {
			t.Fatal(err)
		}
		if err = q.Close(ctx, db, sensitive); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive"} {
			var content string
			err = db.QueryRow("SELECT content FROM "+table+" WHERE id=?", "root'quoted").Scan(&content)
			expected := "node content"
			if sensitive && table == "blocks_fts_case_insensitive" {
				expected = "before"
			}
			if err != nil || content != expected {
				t.Fatal(sensitive, table, content, err)
			}
		}
		if err = bundle.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
