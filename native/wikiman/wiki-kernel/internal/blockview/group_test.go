package blockview

import (
	"context"
	"errors"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
	"wiki-kernel/internal/indexdb"
)

func TestOriginalGroupingSortingCountsAndFailure(t *testing.T) {
	ctx := context.Background()
	root := &ast.Node{ID: "root", Type: ast.NodeDocument}
	root.AppendChild(&ast.Node{ID: "first", Type: ast.NodeParagraph})
	root.AppendChild(&ast.Node{ID: "last", Type: ast.NodeParagraph})
	input := indexdb.BlockSearchResult{Blocks: []indexdb.BlockRecord{{ID: "last", RootID: "root", Content: "__@mark__alpha__mark@__ <x>", Type: "p", Sort: 99}, {ID: "first", RootID: "root", Content: "first", Type: "p", Sort: 1}}}
	services := Services{Strict: true, Tree: func(context.Context, string) (*parse.Tree, error) { return &parse.Tree{Root: root}, nil }, Blocks: func(_ context.Context, ids []string) ([]*indexdb.BlockRecord, error) {
		if !reflect.DeepEqual(ids, []string{"root"}) {
			t.Fatal(ids)
		}
		return []*indexdb.BlockRecord{{ID: "root", RootID: "root", Type: "d", HPath: "/Folder/Doc", IAL: "{: updated=\"20261022\"}"}}, nil
	}, ReferenceCounts: func(_ context.Context, ids []string) (map[string]int, error) {
		return map[string]int{"first": 2, "last": 4}, nil
	}}
	got, e := FullText(ctx, input, 1, 5, false, services)
	if e != nil || len(got) != 1 || len(got[0].Children) != 2 || got[0].Children[0].ID != "first" || got[0].Children[1].RefCount != 4 || got[0].Children[1].Content != "<mark>alpha</mark> &lt;x&gt;" {
		t.Fatal(got, e)
	}
	if input.Blocks[0].Sort != 99 {
		t.Fatal("changed raw input")
	}
	input.Blocks = []indexdb.BlockRecord{{ID: "root", Type: "d", HPath: "/Folder/Doc"}}
	got, e = FullText(ctx, input, 0, 0, false, services)
	if e != nil || got[0].HPath != "/Folder/" {
		t.Fatal(got, e)
	}
	sentinel := errors.New("source failed")
	services.Tree = func(context.Context, string) (*parse.Tree, error) { return nil, sentinel }
	if got, e = FullText(ctx, input, 1, 5, false, services); !errors.Is(e, sentinel) || got != nil {
		t.Fatal(got, e)
	}
	services.ReferenceCounts = func(context.Context, []string) (map[string]int, error) { return nil, sentinel }
	if got, e = FullText(ctx, input, 0, 0, false, services); !errors.Is(e, sentinel) || got != nil {
		t.Fatal(got, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = FullText(canceled, input, 0, 0, false, services); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
