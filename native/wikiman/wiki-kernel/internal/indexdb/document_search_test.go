//go:build fts5

package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"wiki-kernel/conf"
)

func TestDocumentRootSearchOriginalNAMOrderLimitsAndSafeValues(t *testing.T) {
	ctx := context.Background()
	bundle, err := OpenMemory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	rows := []BlockRecord{{ID: "deep", RootID: "deep", Box: "z", Type: "d", HPath: "/needle/deep", Path: "/deep.docj"}, {ID: "shallow", RootID: "shallow", Box: "z", Type: "d", HPath: "/needle", Alias: "aliasword", Memo: "memo", Name: "named"}, {ID: "other", RootID: "other", Box: "a", Type: "d", HPath: "/needle"}, {ID: "quote", RootID: "id'quoted", Box: "z", Type: "d", HPath: "/O'Reilly"}, {ID: "paragraph", RootID: "deep", Box: "z", Type: "p", HPath: "/needle"}}
	if err = Transaction(ctx, db, func(tx *sql.Tx) error { return InsertBlocks(ctx, tx, rows, false, nil) }); err != nil {
		t.Fatal(err)
	}
	s := *conf.NewSearch()
	s.Limit = 2
	got, err := SearchDocumentRoots(ctx, db, s, []string{"needle"}, nil)
	if err != nil || len(got) != 2 || got[0].ID != "shallow" || got[1].ID != "deep" {
		t.Fatal(got, err)
	}
	s.Limit = -1
	got, err = SearchDocumentRoots(ctx, db, s, []string{"needle", "aliasword"}, nil)
	if err != nil || len(got) != 1 || got[0].ID != "shallow" {
		t.Fatal("AND keywords/NAM lost", got, err)
	}
	for _, word := range []string{"named", "aliasword", "memo"} {
		got, err = SearchDocumentRoots(ctx, db, s, []string{word}, nil)
		if err != nil || len(got) != 1 {
			t.Fatal(word, got, err)
		}
	}
	s.Name, s.Alias, s.Memo = false, false, false
	got, err = SearchDocumentRoots(ctx, db, s, []string{"aliasword"}, nil)
	if err != nil || len(got) != 0 {
		t.Fatal("NAM flags ignored", got, err)
	}
	got, err = SearchDocumentRoots(ctx, db, s, []string{"O'Reilly"}, nil)
	if err != nil || len(got) != 1 || got[0].ID != "quote" {
		t.Fatal("apostrophe keyword failed", got, err)
	}
	got, err = SearchDocumentRoots(ctx, db, s, []string{"O'Reilly"}, []string{"id'quoted"})
	if err != nil || len(got) != 0 {
		t.Fatal("quoted exclude failed", got, err)
	}
	got, err = SearchDocumentRoots(ctx, db, s, []string{"%' OR 1=1 --"}, nil)
	if err != nil || len(got) != 0 {
		t.Fatal("keyword became SQL", got, err)
	}
	got, err = SearchDocumentRoots(ctx, db, s, []string{"needle"}, []string{"x') OR 1=1 --"})
	if err != nil || len(got) != 3 {
		t.Fatal("exclude became SQL", got, err)
	}
	got, err = SearchDocumentRoots(ctx, db, s, []string{"need_e"}, nil)
	if err != nil || len(got) != 3 {
		t.Fatal("original LIKE wildcard changed", got, err)
	}
	s.Limit = 0
	got, err = SearchDocumentRoots(ctx, db, s, []string{"needle"}, nil)
	if err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
	if _, err = SearchDocumentRoots(ctx, nil, s, []string{"needle"}, nil); !errors.Is(err, ErrNoDatabase) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if got, err = SearchDocumentRoots(cancelled, db, s, []string{"needle"}, nil); err == nil || got != nil {
		t.Fatal(got, err)
	}
}
