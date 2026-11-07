//go:build fts5

package model

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
	"wiki-kernel/util"
)

func TestHistorySearchActualOwnerDBAndReadOnlyProjection(t *testing.T) {
	c, _, id, _, _ := referenceModelFixture(t)
	ctx := context.Background()
	c.persistenceReadOnly = true
	created := strconv.FormatInt(time.Now().Unix(), 10)
	path := "stamp/20261028140000-refbox1/document.docj"
	q := sql.CaptureLegacyHistoryQueue()
	if e := q.Index(ctx, []indexdb.HistoryEntry{{ID: id, Type: 1, Op: "update", Title: "Owned title", Content: "History needle", Path: path, Created: created}}); e != nil {
		t.Fatal(e)
	}
	if e := q.Flush(ctx, nil); e != nil {
		t.Fatal(e)
	}
	dates, pages, total, e := FullTextSearchHistoryContext(ctx, "needle", "%", "all", 1, 1)
	if e != nil || len(dates) != 1 || dates[0] != created || total != 1 || pages != 1 {
		t.Fatal(dates, pages, total, e)
	}
	rows, e := FullTextSearchHistoryItemsContext(ctx, created, "needle", "%", "all", 1)
	if e != nil || len(rows) != 1 || rows[0].Path != filepath.Join(util.HistoryDir, path) || rows[0].Notebook != "20261028140000-refbox1" {
		t.Fatal(rows, e)
	}
	if _, e = q.DB.Exec("DROP TABLE histories_fts_case_insensitive"); e != nil {
		t.Fatal(e)
	}
	if rows, e = FullTextSearchHistoryItemsContext(ctx, created, "needle", "%", "all", 1); e == nil || rows != nil {
		t.Fatal("SQL failure swallowed", rows, e)
	}
}
