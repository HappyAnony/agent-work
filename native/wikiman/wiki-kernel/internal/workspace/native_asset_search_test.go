//go:build fts5

package workspace

import (
	"context"
	"strings"
	"testing"
	"wiki-kernel/internal/assetquery"
	"wiki-kernel/internal/indexdb"
)

func TestNativeAssetSearchActualEqualIDsOwnerIsolationAndSourceUnchanged(t *testing.T) {
	ctx := context.Background()
	root := fixture(t)
	before := digest(t, root)
	m := open(t, root, Options{})
	const id = "20261104200000-ast0001"
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		if e := l.QueueNativeAssetContents(ctx, []indexdb.AssetContentEntry{{ID: id, Name: page, Ext: ".txt", Path: "assets/same.txt", Content: "Alpha " + page, Updated: 1, Size: 10}}); e != nil {
			t.Fatal(e)
		}
		if e := l.FlushNativeAssetContents(ctx); e != nil {
			t.Fatal(e)
		}
	}
	for _, page := range []string{pageA, pageB} {
		l := getDocLease(t, m, page)
		v, e := l.SearchNativeAssetContents(ctx, assetquery.Request{Query: "Alpha", Page: 1, PageSize: 10})
		if e != nil || v.Matched != 1 || len(v.Contents) != 1 || v.Contents[0].ID != id || v.Contents[0].Name != page {
			t.Fatal(page, v, e)
		}
		detail, e := l.NativeAssetContent(ctx, id, "Alpha", 0)
		if e != nil || !strings.Contains(detail.Content, page) {
			t.Fatal(detail, e)
		}
		detail.Content = "mutated"
		again, e := l.NativeAssetContent(ctx, id, "Alpha", 0)
		if e != nil || again.Content == "mutated" {
			t.Fatal(again, e)
		}
	}
	if digest(t, root) != before {
		t.Fatal("asset search changed source")
	}
}
