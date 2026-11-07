//go:build fts5

package assetquery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalAssetSearchActualModesPagingProjectionAndFailures(t *testing.T) {
	ctx := context.Background()
	b, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	db, _ := b.Database(indexdb.Assets)
	ids := []string{"20261104200000-ast0001", "20261104200001-ast0002", strings.Repeat("a", 64)}
	queue := &indexdb.AssetQueue{}
	if e = queue.Index([]indexdb.AssetContentEntry{{ID: ids[0], Name: "Report", Ext: ".txt", Path: "assets/one.txt", Content: "Alpha <script>\nline", Size: 10, Updated: 1}, {ID: ids[1], Name: "Report", Ext: ".pdf", Path: "assets/two.pdf", Content: "Alpha second", Size: 20, Updated: 2}, {ID: ids[2], Name: "report", Ext: ".txt", Path: "assets/three.txt", Content: "Alpha third", Size: 30, Updated: 3}}); e != nil {
		t.Fatal(e)
	}
	if e = queue.Flush(ctx, db, nil); e != nil {
		t.Fatal(e)
	}
	for _, method := range []int{0, 1, 3} {
		query := "Alpha"
		if method == 3 {
			query = "A[l]pha"
		}
		v, e := Search(ctx, db, Request{Query: query, Method: method, OrderBy: 2, Page: 1, PageSize: 1})
		if e != nil || len(v.Contents) != 1 || v.Matched != 3 || v.PageCount != 3 || v.Contents[0].ID != ids[0] || v.Contents[0].HSize == "" {
			t.Fatal(method, v, e)
		}
	}
	v, e := Search(ctx, db, Request{Query: "Alpha", Types: map[string]bool{".txt": true}, Page: 2, PageSize: 1, OrderBy: 2})
	if e != nil || v.Matched != 2 || len(v.Contents) != 1 || v.Contents[0].ID != ids[2] {
		t.Fatal(v, e)
	}
	detail, e := Detail(ctx, db, v.Contents[0].ID, "Alpha", 0)
	if e != nil || detail == nil || detail.Content != "<mark>Alpha</mark> third" {
		t.Fatal(detail, e)
	}
	v, e = Search(ctx, db, Request{Query: "Alpha", Types: map[string]bool{}, Page: 1, PageSize: 10})
	if e != nil || v.Matched != 0 || len(v.Contents) != 0 {
		t.Fatal(v, e)
	}
	v, e = Search(ctx, db, Request{Query: "SELECT * FROM asset_contents_fts_case_insensitive WHERE name='Report'", Method: 2, Page: 1, PageSize: 1})
	if e != nil || v.Matched != 2 || v.PageCount != 2 || len(v.Contents) != 1 {
		t.Fatal("SQL count changed literal case", v, e)
	}
	for _, method := range []int{0, 1, 2, 3} {
		query := "Alpha"
		if method == 2 {
			query = "SELECT * FROM asset_contents_fts_case_insensitive"
		}
		if method == 3 {
			query = "A[l]pha"
		}
		v, e := Detail(ctx, db, ids[0], query, method)
		if e != nil || v == nil || !strings.Contains(v.Content, "&lt;script&gt;<br>line") {
			t.Fatal(method, v, e)
		}
		if method != 2 && !strings.Contains(v.Content, "<mark>Alpha</mark>") {
			t.Fatal("missing detail highlight", method, v)
		}
	}
	for _, q := range []Request{{Query: "DELETE FROM asset_contents_fts_case_insensitive", Method: 2, Page: 1, PageSize: 10}, {Query: "SELECT * FROM missing", Method: 2, Page: 1, PageSize: 10}, {Query: "[", Method: 3, Page: 1, PageSize: 10}, {Query: "x", Page: 1, PageSize: 0}} {
		v, e := Search(ctx, db, q)
		if e == nil || v.Contents != nil {
			t.Fatal("invalid query returned partial success", q, v, e)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = Detail(canceled, db, ids[0], "", 0); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = db.Exec("DROP TABLE asset_contents_fts_case_insensitive"); e != nil {
		t.Fatal(e)
	}
	if v, e := Search(ctx, db, Request{Query: "Alpha", Page: 1, PageSize: 1}); e == nil || v.Contents != nil {
		t.Fatal(v, e)
	}
}
