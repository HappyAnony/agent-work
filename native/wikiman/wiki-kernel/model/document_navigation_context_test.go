//go:build fts5

package model

import (
	"bytes"
	"context"
	"os"
	"testing"
	"wiki-kernel/conf"
	"wiki-kernel/internal/docquery"
)

func TestDocumentNavigationActualReadOnlySourceAndIndexFailures(t *testing.T) {
	c, r, def, file, before := referenceModelFixture(t)
	c.Export = conf.NewExport()
	c.persistenceReadOnly = true
	ctx := context.Background()
	const box = "20261028140000-refbox1"
	const root = "20261028140001-refdoc1"
	for _, q := range []docquery.PathRequest{
		{Mode: "path", Box: box, Path: "/" + root + ".docj"},
		{Mode: "id", ID: def}, {Mode: "full", ID: def}, {Mode: "location", ID: def},
		{Mode: "batch", Paths: []string{"/" + root + ".docj", "/" + root + ".docj", "/missing.docj"}},
		{Mode: "ids", Box: box, HPath: "/reference fixture"},
	} {
		out, e := DocumentPathsContext(ctx, q)
		if e != nil {
			t.Fatal(q, out, e)
		}
		if q.Mode == "location" && (out.Path != "/"+root+".docj" || out.Box != box) {
			t.Fatal(out)
		}
		if q.Mode == "batch" && len(out.Paths) != 1 {
			t.Fatal(out)
		}
	}
	out, e := SearchDocsByKeywordContext(ctx, "reference fixture", false, nil)
	if e != nil || len(out) != 2 {
		t.Fatal(out, e)
	}
	out, e = SearchDocsByKeywordContext(ctx, "reference fixture", false, []string{root})
	if e != nil || len(out) != 1 {
		t.Fatal(out, e)
	}
	raw, e := os.ReadFile(file)
	if e != nil || !bytes.Equal(raw, before) {
		t.Fatal("navigation changed source", e)
	}
	if _, e = r.main.DB.Exec("DROP TABLE blocks"); e != nil {
		t.Fatal(e)
	}
	if out, e = SearchDocsByKeywordContext(ctx, "reference", false, nil); e == nil || out != nil {
		t.Fatal("SQL error swallowed", out, e)
	}
}
