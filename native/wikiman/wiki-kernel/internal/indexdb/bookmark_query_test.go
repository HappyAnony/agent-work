//go:build fts5

package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
)

func TestBookmarkQueryPreservesBroadCandidatesCompleteRowsAndErrors(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, err := bundle.Database(Main)
	if err != nil {
		t.Fatal(err)
	}
	row := BlockRecord{ID: "block", ParentID: "parent", RootID: "root", Hash: "hash", Box: "owner", Path: "/doc.docj", HPath: "/Title", Name: "name", Alias: "alias", Memo: "memo", Tag: "tag", Content: "body", FContent: "first", Markdown: "markdown", Length: 4, Type: "p", SubType: "sub", IAL: `bookmark="O'ne %_"`, Sort: 13, Created: "20261026", Updated: "20261027"}
	if err = Transaction(ctx, db, func(tx *sql.Tx) error {
		return InsertBlocks(ctx, tx, []BlockRecord{row, {ID: "broad", RootID: "root", IAL: `custom-bookmark="other"`}, {ID: "not-a-bookmark", RootID: "root", IAL: `name="bookmark"`}}, false, nil)
	}); err != nil {
		t.Fatal(err)
	}
	got, err := BookmarkBlocks(ctx, db)
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}
	var found bool
	for _, b := range got {
		if b.ID == row.ID {
			found = true
			if !reflect.DeepEqual(b, row) {
				t.Fatal("incomplete row", b, row)
			}
		}
	}
	if !found {
		t.Fatal("row missing")
	}
	if _, err = BookmarkBlocks(ctx, nil); !errors.Is(err, ErrNoDatabase) {
		t.Fatal("missing DB accepted", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if got, err = BookmarkBlocks(cancelled, db); err == nil || got != nil {
		t.Fatal("cancelled read returned success/partial rows", got, err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err = BookmarkBlocks(ctx, db); err == nil || got != nil {
		t.Fatal("closed DB returned success", got, err)
	}
}

func TestBookmarkLabelsOriginalSubstringSortDedupAndMalformedBounds(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, err := bundle.Database(Main)
	if err != nil {
		t.Fatal(err)
	}
	rows := []BlockRecord{{ID: "a", IAL: `{: bookmark="z"}`}, {ID: "b", IAL: `{: bookmark="A"}`}, {ID: "c", IAL: `{: bookmark="A"}`}, {ID: "d", IAL: `{: custom-bookmark="custom"}`}, {ID: "e", IAL: `{: bookmark=""}`}, {ID: "f", IAL: `{: name="bookmark"}`}}
	if err = Transaction(ctx, db, func(tx *sql.Tx) error { return InsertBlocks(ctx, tx, rows, false, nil) }); err != nil {
		t.Fatal(err)
	}
	got, err := BookmarkLabels(ctx, db)
	if err != nil || !reflect.DeepEqual(got, []string{"A", "custom", "z"}) {
		t.Fatal(got, err)
	}
	if _, err = db.ExecContext(ctx, "UPDATE blocks SET ial=? WHERE id='a'", "bookmark="); err != nil {
		t.Fatal(err)
	}
	if got, err = BookmarkLabels(ctx, db); err == nil || got != nil {
		t.Fatal("corrupt IAL produced success", got, err)
	}
}
