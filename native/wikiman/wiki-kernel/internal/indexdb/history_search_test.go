//go:build fts5

package indexdb

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestHistorySearchOriginalModesPagingBoundValuesAndErrors(t *testing.T) {
	ctx := context.Background()
	b, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	db, _ := b.Database(History)
	now := time.Now()
	created := strconv.FormatInt(now.Unix(), 10)
	var queue HistoryQueue
	items := []HistoryEntry{}
	for i := range 40 {
		items = append(items, HistoryEntry{ID: fmt.Sprint(i), Title: "O'Reilly title", Content: "body needle", Op: "update", Type: 1, Path: "stamp/20261028140000-refbox1/doc.docj", Created: strconv.FormatInt(now.Unix()-int64(i), 10)})
	}
	items = append(items, HistoryEntry{ID: "asset", Title: "asset title", Content: "asset needle", Op: "update", Type: 2, Path: "stamp/assets/file.txt", Created: created})
	items = append(items, HistoryEntry{ID: "expired", Title: "expired", Op: "update", Path: "stamp/box/doc.docj", Created: strconv.FormatInt(now.Add(-40*24*time.Hour).Unix(), 10)})
	if e = queue.Index(items); e != nil {
		t.Fatal(e)
	}
	if e = queue.Flush(ctx, db, nil); e != nil {
		t.Fatal(e)
	}
	q := HistorySearchRequest{Query: "O'Reilly", Type: 0, Page: 1, Operation: "all", Box: "%", RetentionDays: 30, Now: now}
	out, e := SearchHistory(ctx, db, q)
	if e != nil || out.Total != 40 || out.Pages != 2 || len(out.Created) != 32 {
		t.Fatal(out, e)
	}
	q.Page = 2
	out, e = SearchHistory(ctx, db, q)
	if e != nil || len(out.Created) != 8 {
		t.Fatal(out, e)
	}
	q.Page = 999999999
	out, e = SearchHistory(ctx, db, q)
	if e != nil || len(out.Created) != 0 || out.Total != 40 {
		t.Fatal(out, e)
	}
	q.Page = 1
	q.Query = "needle"
	q.Type = 0
	out, e = SearchHistory(ctx, db, q)
	if e != nil || out.Total != 0 {
		t.Fatal(out, e)
	}
	q.Type = 1
	out, e = SearchHistory(ctx, db, q)
	if e != nil || out.Total != 40 {
		t.Fatal(out, e)
	}
	q.Type = 2
	rows, e := SearchHistoryItems(ctx, db, q, created)
	if e != nil || len(rows) != 1 || rows[0].ID != "asset" {
		t.Fatal(rows, e)
	}
	q.Type = 3
	q.Query = "0"
	out, e = SearchHistory(ctx, db, q)
	if e != nil || out.Total != 1 {
		t.Fatal(out, e)
	}
	q.Query = "0' OR 1=1 --"
	out, e = SearchHistory(ctx, db, q)
	if e != nil || out.Total != 0 {
		t.Fatal(out, e)
	}
	q.Query = ""
	q.Type = 1
	q.Operation = "update' OR 1=1 --"
	out, e = SearchHistory(ctx, db, q)
	if e != nil || out.Total != 0 {
		t.Fatal(out, e)
	}
	q.Operation = "all"
	if e = queue.Clear(); e != nil {
		t.Fatal(e)
	}
	if e = queue.Index([]HistoryEntry{{ID: "after-clear", Title: "new", Path: "stamp/box/doc.docj", Created: created}}); e != nil {
		t.Fatal(e)
	}
	if e = queue.Flush(ctx, db, nil); e != nil {
		t.Fatal(e)
	}
	out, e = SearchHistory(ctx, db, q)
	if e != nil || out.Total != 1 {
		t.Fatal("clear lost FIFO order", out, e)
	}
	if _, e = db.Exec("DROP TABLE histories_fts_case_insensitive"); e != nil {
		t.Fatal(e)
	}
	if out, e = SearchHistory(ctx, db, q); e == nil || out.Created != nil || out.Total != 0 {
		t.Fatal("error returned partial", out, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if rows, e = SearchHistoryItems(canceled, db, q, created); e == nil || rows != nil {
		t.Fatal(rows, e)
	}
}
