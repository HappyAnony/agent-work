//go:build fts5

package model

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/sql"
)

func TestAssetSearchActualCapturedDBReadOnlyAndSQLFailure(t *testing.T) {
	c, _, _, file, before := referenceModelFixture(t)
	ctx := context.Background()
	const id = "20261104200000-ast0001"
	q := sql.CaptureLegacyAssetQueue()
	if e := q.Index(ctx, []indexdb.AssetContentEntry{{ID: id, Name: "Report", Ext: ".txt", Path: "assets/report.txt", Size: 10, Content: "Alpha & content"}}); e != nil {
		t.Fatal(e)
	}
	if e := q.Flush(ctx, nil); e != nil {
		t.Fatal(e)
	}
	c.persistenceReadOnly = true
	rows, count, pages, e := FullTextSearchAssetContentContext(ctx, "Alpha", nil, 0, 0, 1, 10)
	if e != nil || count != 1 || pages != 1 || len(rows) != 1 || rows[0].ID != id {
		t.Fatal(rows, count, pages, e)
	}
	detail, e := GetAssetContentContext(ctx, id, "Alpha", 0)
	if e != nil || detail == nil || !strings.Contains(detail.Content, "<mark>Alpha</mark>") {
		t.Fatal(detail, e)
	}
	if raw, e := os.ReadFile(file); e != nil || !bytes.Equal(raw, before) {
		t.Fatal("asset read changed source", e)
	}
	if _, e = q.DB.Exec("DROP TABLE asset_contents_fts_case_insensitive"); e != nil {
		t.Fatal(e)
	}
	if rows, _, _, e = FullTextSearchAssetContentContext(ctx, "Alpha", nil, 0, 0, 1, 10); e == nil || rows != nil {
		t.Fatal("DB error swallowed", rows, e)
	}
}
