//go:build fts5

package workspace

import (
	"context"
	"strings"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestNativeTagNavigationRealOwnerSpansBlocksAndNoSourceWrites(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{Configuration: originalProjectionConfiguration})
	for _, page := range []string{pageA, pageB} {
		l, err := m.Acquire(ctx, storeID, page)
		if err != nil {
			t.Fatal(err)
		}
		if err = l.QueueNativeTree(ctx, boxID, docID, "index"); err != nil {
			t.Fatal(err)
		}
		if err = l.FlushNativeMainQueue(ctx); err != nil {
			t.Fatal(err)
		}
		if err = l.WithNativeIndexes(ctx, func(b *indexdb.Bundle) error {
			db, e := b.Database(indexdb.Main)
			if e != nil {
				return e
			}
			_, e = db.ExecContext(ctx, "INSERT INTO spans(id,block_id,root_id,box,path,content,markdown,type,ial) VALUES(?,?,?,?,?,?,?,?,?)", "navigation-tag", blockID, docID, boxID, "/"+docID+".docj", page+" tag", "#"+page+" tag#", "tag", "")
			return e
		}); err != nil {
			t.Fatal(err)
		}
		got, err := l.NativeTagBlocks(ctx, page+" tag")
		if err != nil || len(got[page+" tag"]) != 1 {
			t.Fatal(got, err)
		}
		want := "Synthetic"
		if page == pageB {
			want = "Workspace B"
		}
		if !strings.Contains(got[page+" tag"][0].Content, want) {
			t.Fatal("foreign owner block", got)
		}
		got[page+" tag"][0].Content = "caller mutation"
		again, err := l.NativeTagBlocks(ctx, page+" tag")
		if err != nil || !strings.Contains(again[page+" tag"][0].Content, want) {
			t.Fatal(again, err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if got, err = l.NativeTagBlocks(cancelled, page); err == nil || got != nil {
			t.Fatal("cancelled query returned partial", got, err)
		}
		l.Release()
		if got, err = l.NativeTagBlocks(ctx, page); err == nil || got != nil {
			t.Fatal("released owner accepted", got, err)
		}
	}
	if digest(t, root) != before {
		t.Fatal("navigation changed source")
	}
}
