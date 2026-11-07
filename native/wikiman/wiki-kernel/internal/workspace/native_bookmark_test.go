//go:build fts5

package workspace

import (
	"context"
	"database/sql"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestNativeBookmarksOwnIndexBroadCandidatesAndLifecycle(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	for _, page := range []string{pageA, pageB} {
		l, err := m.Acquire(ctx, storeID, page)
		if err != nil {
			t.Fatal(err)
		}
		rows := []indexdb.BlockRecord{
			{ID: blockID, RootID: docID, Box: boxID, IAL: `bookmark="One"`, Content: page},
			{ID: "other", RootID: docID, Box: boxID, IAL: `custom-bookmark="One/sub"`, Content: page},
			{ID: "ordinary", RootID: docID, Box: boxID, IAL: `name="bookmark"`, Content: page},
		}
		if err = l.WithNativeTransaction(ctx, indexdb.Main, func(tx *sql.Tx) error { return indexdb.InsertBlocks(ctx, tx, rows, false, nil) }); err != nil {
			t.Fatal(err)
		}
		got, err := l.NativeBookmarkBlocks(ctx)
		if err != nil || len(got) != 2 {
			t.Fatal(got, err)
		}
		for _, b := range got {
			if b.Content != page {
				t.Fatal("foreign index", b)
			}
		}
		got[0].Content = "caller mutation"
		again, err := l.NativeBookmarkBlocks(ctx)
		if err != nil || again[0].Content != page {
			t.Fatal("returned row aliases owner", again, err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err = l.NativeBookmarkBlocks(cancelled); err == nil {
			t.Fatal("cancelled read succeeded")
		}
		l.Release()
		if _, err = l.NativeBookmarkBlocks(ctx); err == nil {
			t.Fatal("released lease read succeeded")
		}
	}
	if digest(t, root) != before {
		t.Fatal("bookmark query wrote source")
	}
}
