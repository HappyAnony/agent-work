//go:build fts5

package indexdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"wiki-kernel/internal/nativeparser"
)

func TestOriginalUpperReferenceQueriesAndText(t *testing.T) {
	ctx := context.Background()
	bundle, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	q := References{DB: db}
	_, e = db.Exec(`INSERT INTO blocks(id,parent_id,root_id,type,content,name,markdown) VALUES('root','','root','d','title','',''),('def','root','root','p','body','',''),('child','def','root','p','child','',''),('ref','','ref','p','reference','',''),('list','','root','l','','','- first item');`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec("UPDATE blocks SET hash='',box='',path='',hpath='',alias='',memo='',tag='',fcontent='',length=0,subtype='',ial='',sort=0,created='',updated=''")
	if e != nil {
		t.Fatal(e)
	}
	e = Transaction(ctx, db, func(tx *sql.Tx) error {
		return InsertBlockReferences(ctx, tx, []RefRecord{{ID: "1", DefBlockID: "def", DefBlockRootID: "root", BlockID: "ref", RootID: "ref"}, {ID: "2", DefBlockID: "child", DefBlockRootID: "root", BlockID: "ref2", RootID: "ref"}, {ID: "3", DefBlockID: "quote'", DefBlockRootID: "root", BlockID: "ref3"}})
	})
	if e != nil {
		t.Fatal(e)
	}
	r, e := q.ByDefinition(ctx, "def", true)
	if e != nil || len(r) != 2 {
		t.Fatal(r, e)
	}
	r, e = q.ByPair(ctx, "def", "ref")
	if e != nil || len(r) != 1 {
		t.Fatal(r, e)
	}
	ids, e := q.RefIDs(ctx, "root", true)
	if e != nil || len(ids) != 3 {
		t.Fatal(ids, e)
	}
	children, e := q.ChildDefinitions(ctx, "root")
	if e != nil || len(children) != 3 {
		t.Fatal(children, e)
	}
	if !reflect.DeepEqual(children, map[string][]string{"ref": {"def"}, "ref2": {"child"}, "ref3": {"quote'"}}) {
		t.Fatal("reference/definition direction reversed", children)
	}
	if _, e = db.ExecContext(ctx, "INSERT INTO refs(id,def_block_id,def_block_root_id,block_id) VALUES('4','child','root','ref')"); e != nil {
		t.Fatal(e)
	}
	children, e = q.ChildDefinitions(ctx, "root")
	if e != nil || len(children) != 3 || !reflect.DeepEqual(children["ref"], []string{"def", "child"}) {
		t.Fatal(children, e)
	}
	if _, e = db.ExecContext(ctx, "DELETE FROM refs WHERE id='4'"); e != nil {
		t.Fatal(e)
	}
	roots, e := q.DefinitionRoots(ctx, "ref")
	if e != nil || len(roots) != 1 || roots[0].Content != "title" {
		t.Fatal(roots, e)
	}
	r, e = q.ByDefinition(ctx, "quote'", false)
	if e != nil || len(r) != 1 {
		t.Fatal(r, e)
	}
	parser := nativeparser.NewIndex(nativeparser.Defaults())
	read := func(ctx context.Context, id string) (BlockRecord, bool, error) {
		b, e := scanBlock(db.QueryRowContext(ctx, "SELECT * FROM blocks WHERE id=?", id))
		if errors.Is(e, sql.ErrNoRows) {
			return BlockRecord{}, false, nil
		}
		return b, e == nil, e
	}
	for id, want := range map[string]string{"root": "title", "def": "body", "list": "first item", "missing": "block not found", "assets/a": "assets/a"} {
		got, e := ReferenceText(ctx, id, parser, read)
		if e != nil || got != want {
			t.Fatal(id, got, e)
		}
	}
	_, e = db.Exec("UPDATE blocks SET parent_id='child' WHERE id='def'")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = q.BlockChildren(ctx, "def"); e == nil {
		t.Fatal("cycle not rejected")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = q.ByDefinition(canceled, "def", false); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func TestReferenceMatchingKeywordsRecentPairsAndLargeRootLists(t *testing.T) {
	ctx := context.Background()
	bundle, e := OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer bundle.Close()
	db, _ := bundle.Database(Main)
	q := References{DB: db}
	blocks := []BlockRecord{{ID: "root'", Type: "d", Content: "AGI", Name: "Doc,Name", Alias: "AI, AGI"}, {ID: "def", Type: "p", Name: "AGI", Alias: "partAGI", Content: "text"}, {ID: "ref", RootID: "root'", Type: "p", Content: "referencing"}}
	refs := []RefRecord{{ID: "1", DefBlockID: "def", DefBlockRootID: "root'", BlockID: "ref", RootID: "root'", Content: "AGI"}, {ID: "2", DefBlockID: "root'", DefBlockRootID: "root'", BlockID: "ref", RootID: "root'", Content: "AGI"}}
	e = Transaction(ctx, db, func(tx *sql.Tx) error {
		if e := InsertBlocks(ctx, tx, blocks, false, nil); e != nil {
			return e
		}
		return InsertBlockReferences(ctx, tx, refs)
	})
	if e != nil {
		t.Fatal(e)
	}
	ids, e := q.DefinitionIDsByText(ctx, "AGI", []string{"def"}, true)
	if e != nil || len(ids) != 1 || ids[0] != "root'" {
		t.Fatal(ids, e)
	}
	keywords, e := q.VirtualKeywords(ctx, true, true, true, true, nil, nil)
	if e != nil || len(keywords) != 6 {
		t.Fatal(keywords, e)
	}
	recent, e := q.Recent(ctx, true, "", nil)
	if e != nil || len(recent) != 1 || recent[0].DefBlockID != "root'" {
		t.Fatal(recent, e)
	}
	recent, e = q.Recent(ctx, false, "('p')", []string{"r.content='AGI'"})
	if e != nil || len(recent) != 1 || recent[0].DefBlockID != "def" {
		t.Fatal(recent, e)
	}
	pairs, e := q.DefinitionPairs(ctx, "ref.id='ref'", -1)
	if e != nil || len(pairs) != 2 {
		t.Fatal(pairs, e)
	}
	var roots []string
	for i := 0; i < 1025; i++ {
		roots = append(roots, fmt.Sprintf("missing-%d", i))
	}
	roots = append(roots, "root'", "root'")
	groups, e := q.ReferringRoots(ctx, roots)
	if e != nil || len(groups["root'"]) != 2 {
		t.Fatal(groups, e)
	}
	if _, e = q.Recent(ctx, false, "broken", nil); e == nil {
		t.Fatal("invalid policy silently succeeded")
	}
}
