//go:build fts5

package workspace

import (
	"context"
	"strconv"
	"testing"
	"time"
	"wiki-kernel/internal/indexdb"
)

func TestNativeHistorySearchRealSameIDOwners(t *testing.T) {
	ctx := context.Background()
	root := getDocSource(t)
	before := digest(t, root)
	m := open(t, root, getDocOptions())
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		if e := l.QueueNativeHistories(ctx, []indexdb.HistoryEntry{{ID: blockID, Type: 1, Op: "update", Title: page, Content: "Owned history", Created: stamp, Path: "stamp/" + boxID + "/" + docID + ".docj"}}); e != nil {
			t.Fatal(e)
		}
		if e := l.FlushNativeHistories(ctx); e != nil {
			t.Fatal(e)
		}
		q := indexdb.HistorySearchRequest{Type: 1, Page: 1, Query: "Owned", Box: "%", Operation: "all"}
		out, e := l.NativeHistorySearch(ctx, q)
		if e != nil || out.Total != 1 || len(out.Created) != 1 {
			t.Fatal(out, e)
		}
		rows, e := l.NativeHistorySearchItems(ctx, q, stamp)
		if e != nil || len(rows) != 1 || rows[0].Title != page {
			t.Fatal(page, rows, e)
		}
		rows[0].Title = "changed"
		again, e := l.NativeHistorySearchItems(ctx, q, stamp)
		if e != nil || again[0].Title != page {
			t.Fatal(again, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("history query published source")
	}
}
