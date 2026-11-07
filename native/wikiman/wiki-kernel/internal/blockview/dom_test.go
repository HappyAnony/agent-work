//go:build fts5

package blockview

import (
	"context"
	"database/sql"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"strings"
	"testing"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

func TestDOMOriginalEmbedIDsActualSQLFailureAndCancellation(t *testing.T) {
	ctx := context.Background()
	b, e := indexdb.OpenMemory(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	db, _ := b.Database(indexdb.Main)
	const embed = "20261104180000-emb0001"
	const def = "20261104180001-def0001"
	if e = indexdb.Transaction(ctx, db, func(tx *sql.Tx) error {
		return indexdb.InsertBlocks(ctx, tx, []indexdb.BlockRecord{{ID: def, RootID: def, Type: "p", Content: "actual embedded text"}}, false, nil)
	}); e != nil {
		t.Fatal(e)
	}
	parser := nativeparser.NewEditor(nativeparser.EditorOptions{Markdown: nativeparser.Defaults()})
	makeTree := func(id string) *parse.Tree {
		root := &ast.Node{ID: "20261104180002-doc0001", Type: ast.NodeDocument}
		p := &ast.Node{ID: id, Type: ast.NodeParagraph}
		p.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("actual embedded text")})
		root.AppendChild(p)
		if id == embed {
			p.Type = ast.NodeBlockQueryEmbed
			for c := p.FirstChild; c != nil; {
				next := c.Next
				c.Unlink()
				c = next
			}
			p.AppendChild(&ast.Node{Type: ast.NodeBlockQueryEmbedScript, Tokens: []byte("SELECT * FROM blocks WHERE id='" + def + "'")})
		}
		return &parse.Tree{Root: root, ID: root.ID}
	}
	s := DOMServices{Parser: parser, Trees: func(c context.Context, ids []string) (map[string]*parse.Tree, error) {
		return map[string]*parse.Tree{embed: makeTree(embed)}, c.Err()
	}, Load: func(c context.Context, id string) (*parse.Tree, error) { return makeTree(id), c.Err() }, Select: func(c context.Context, stmt string) ([]indexdb.BlockRecord, error) {
		return indexdb.SelectReadBlockRows(c, db, stmt, 1, 64)
	}}
	out, e := DOMs(ctx, []string{embed}, true, s)
	if e != nil || !strings.Contains(out[embed], def) || !strings.Contains(out[embed], "actual embedded text") {
		t.Fatal(out, e)
	}
	if _, e = db.Exec("DROP TABLE blocks"); e != nil {
		t.Fatal(e)
	}
	if _, e = DOMs(ctx, []string{embed}, true, s); e == nil {
		t.Fatal("query error swallowed")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = DOMs(canceled, []string{embed}, true, s); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	want := errors.New("source error")
	s.Load = func(context.Context, string) (*parse.Tree, error) { return nil, want }
	s.Select = func(context.Context, string) ([]indexdb.BlockRecord, error) {
		return []indexdb.BlockRecord{{ID: def, Type: "p"}}, nil
	}
	if _, e = DOMs(ctx, []string{embed}, true, s); !errors.Is(e, want) {
		t.Fatal("source error swallowed", e)
	}
}
