//go:build fts5

package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"wiki-kernel/conf"
)

func TestDocumentKeywordsFullFieldsOriginalORAndQuotedRoot(t *testing.T) {
	ctx := context.Background()
	bundle, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	records := []BlockRecord{{ID: "one", RootID: "root'quoted", Type: "p", Content: "Alpha O'Reilly", Name: "NameToken", Alias: "AliasToken", Memo: "MemoToken", IAL: "IALToken", HPath: "/Folder"}, {ID: "two", RootID: "root'quoted", Type: "p", Content: "Beta"}, {ID: "other", RootID: "outside", Type: "p", Content: "Foreign"}}
	if e = Transaction(ctx, db, func(tx *sql.Tx) error { return InsertBlocks(ctx, tx, records, false, nil) }); e != nil {
		t.Fatal(e)
	}
	c := *conf.NewSearch()
	c.Name = true
	c.Alias = true
	c.Memo = true
	c.IAL = true
	for _, tc := range []struct {
		query  string
		method int
		want   []string
	}{{"Alpha", 0, []string{"Alpha"}}, {"Alpha", 1, []string{"Alpha"}}, {"Alp.a", 3, []string{"Alpha"}}, {"Alpha Beta", 0, []string{"Alpha", "Beta"}}, {"O'Reilly", 0, []string{"O'Reilly"}}, {"NameToken", 0, []string{"NameToken"}}, {"AliasToken", 1, []string{"AliasToken"}}, {"MemoToken", 0, []string{"MemoToken"}}, {"IALToken", 1, []string{"IALToken"}}, {"Foreign", 0, nil}, {"Alpha", 2, nil}} {
		got, e := DocumentKeywords(ctx, db, c, tc.query, map[string]bool{"paragraph": true}, tc.method, "root'quoted")
		if e != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal(tc, got, e)
		}
	}
	if _, e = DocumentKeywords(ctx, db, c, "[", nil, 3, "root'quoted"); e == nil {
		t.Fatal("invalid regex accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = DocumentKeywords(canceled, db, c, "Alpha", nil, 0, "root'quoted"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func TestDocumentKeywordsLimitsFTSAndRegexpTo256Rows(t *testing.T) {
	ctx := context.Background()
	bundle, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	rows := make([]BlockRecord, 257)
	for i := range rows {
		rows[i] = BlockRecord{ID: fmt.Sprint(i), RootID: "root", Type: "p", Content: fmt.Sprintf("limittoken%c%c", 'a'+i/26, 'a'+i%26)}
	}
	rows[256].Name = "specialboundary"
	if e = Transaction(ctx, db, func(tx *sql.Tx) error { return InsertBlocks(ctx, tx, rows, false, nil) }); e != nil {
		t.Fatal(e)
	}
	c := *conf.NewSearch()
	c.Name = true
	keys, e := DocumentKeywords(ctx, db, c, "limittoken* specialboundary", nil, 1, "root")
	// The inherited tokenizer highlights the matched prefix, so dedup leaves
	// one keyword. Row 257's distinct name must not leak past the SQL limit.
	if e != nil || !reflect.DeepEqual(keys, []string{"limittoken"}) {
		t.Fatal(keys, e)
	}
	keys, e = DocumentKeywords(ctx, db, c, "limittoken..", nil, 3, "root")
	if e != nil || len(keys) != 256 {
		t.Fatal(len(keys), e)
	}
}
