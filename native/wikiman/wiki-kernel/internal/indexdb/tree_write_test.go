//go:build fts5

package indexdb

import (
	"context"
	"testing"
)

func queuedTree(n int) TreeRecords {
	rows := treeBlockRows(n)
	ret := TreeRecords{RootID: rows[0].RootID, Box: rows[0].Box, Path: rows[0].Path, Blocks: rows}
	ret.Spans = []SpanRecord{{ID: "span", BlockID: rows[0].ID, RootID: ret.RootID, Box: ret.Box, Path: ret.Path, Content: "span text"}}
	ret.Assets = []AssetRecord{{ID: "asset", BlockID: rows[0].ID, RootID: ret.RootID, Box: ret.Box, DocPath: ret.Path, Path: "assets/a", Hash: "hash"}}
	ret.Attributes = []AttributeRecord{{ID: "attr", BlockID: rows[0].ID, RootID: ret.RootID, Box: ret.Box, Path: ret.Path, Name: "custom-test", Value: "original"}}
	ret.Refs = []RefRecord{{ID: "ref", BlockID: rows[0].ID, RootID: ret.RootID, Box: ret.Box, Path: ret.Path}}
	ret.Annotations = []AnnotationRecord{{ID: "annotation", BlockID: rows[0].ID, RootID: ret.RootID, Box: ret.Box, Path: ret.Path}}
	return ret
}
func TestFullTreeQueueCopiesAllRowsAndDoesNotPublishRolledBackBatches(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	q := &ContentQueue{}
	tree := queuedTree(513)
	if err = q.Enqueue(MainMutation{Action: "index", ID: tree.RootID, Tree: &tree}); err != nil {
		t.Fatal(err)
	}
	tree.Blocks[0].Content = "mutated"
	tree.Spans[0].Content = "mutated"
	tree.Assets[0].Hash = "mutated"
	tree.Attributes[0].Value = "mutated"
	tree.Refs[0].ID = "mutated"
	tree.Annotations[0].ID = "mutated"
	tree.IgnoreLines = []string{"*"}
	if _, err = db.Exec("CREATE TRIGGER fail_attrs BEFORE INSERT ON attributes BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	var progress []MainProgress
	observer := func(p MainProgress) { progress = append(progress, p) }
	if err = q.FlushMainProgress(ctx, db, false, observer); err == nil {
		t.Fatal("missing failure")
	}
	if len(progress) != 0 || q.Pending() != 1 {
		t.Fatal(progress, q.Pending())
	}
	if _, err = db.Exec("DROP TRIGGER fail_attrs"); err != nil {
		t.Fatal(err)
	}
	if err = q.FlushMainProgress(ctx, db, false, observer); err != nil {
		t.Fatal(err)
	}
	if len(progress) != 3 || !progress[0].TreeBatch || progress[0].BlockCount != 512 || progress[1].BlockCount != 1 || progress[2].TreeBatch {
		t.Fatal(progress)
	}
	for table, want := range map[string]string{"blocks": "Searchable 内容", "spans": "span text", "assets": "hash", "attributes": "original", "refs": "ref", "file_annotation_refs": "annotation"} {
		column := map[string]string{"blocks": "content", "spans": "content", "assets": "hash", "attributes": "value", "refs": "id", "file_annotation_refs": "id"}[table]
		var got string
		if err = db.QueryRow("SELECT " + column + " FROM " + table + " LIMIT 1").Scan(&got); err != nil || got != want {
			t.Fatal(table, got, want, err)
		}
	}
}
func TestFullTreeUpsertIgnoreKeepsOriginalPostDeletionSemantics(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	q := &ContentQueue{}
	tree := queuedTree(2)
	if err = q.Enqueue(MainMutation{Action: "index", ID: tree.RootID, Tree: &tree}); err != nil {
		t.Fatal(err)
	}
	if err = q.Flush(ctx, db, false); err != nil {
		t.Fatal(err)
	}
	tree.Blocks[1].Hash = "changed"
	tree.IgnoreLines = []string{"*"}
	if err = q.Enqueue(MainMutation{Action: "upsert", ID: tree.RootID, Tree: &tree}); err != nil {
		t.Fatal(err)
	}
	if err = q.Flush(ctx, db, false); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"blocks", "blocks_fts", "blocks_fts_case_insensitive", "spans", "assets", "attributes", "refs", "file_annotation_refs"} {
		var count int
		want := 0
		if table == "blocks" || table == "blocks_fts" || table == "blocks_fts_case_insensitive" {
			want = 1
		}
		if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatal(table, count, want, err)
		}
	}
}
