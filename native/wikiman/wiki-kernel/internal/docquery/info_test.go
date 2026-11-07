package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"testing"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

func infoFixture() (*parse.Tree, InfoServices) {
	t := &parse.Tree{Root: &ast.Node{Type: ast.NodeDocument, ID: "root"}}
	t.Root.SetIALAttr("title", "Title & value")
	t.Root.SetIALAttr("custom-avs", "first,second")
	t.Root.SetIALAttr("icon", "book")
	s := InfoServices{Wait: func(context.Context) error { return nil }, Trees: func(_ context.Context, ids []string) (map[string]*parse.Tree, error) {
		return map[string]*parse.Tree{"block": t}, nil
	}, Exists: func(_ context.Context, id string) (bool, error) { return id == "valid", nil }, References: func(context.Context, string, string) ([]string, error) { return []string{"list", "list"}, nil }, AVName: func(_ context.Context, id string) (string, error) {
		if id == "first" {
			return "First", nil
		}
		return "", nil
	}, DefaultAVName: func(context.Context) (string, error) { return "Database", nil }, Subfiles: func(context.Context, *parse.Tree) (int, error) { return 2, nil }}
	return t, s
}
func TestDocumentInfoPayloadScrollFlagsAndCopies(t *testing.T) {
	ctx := context.Background()
	tree, s := infoFixture()
	ids := []string{"block", "absent", "block"}
	for _, scroll := range []struct {
		raw  string
		keep bool
	}{{`{"zoomInId":"valid","startId":"bad"}`, true}, {`{"startId":"bad","endId":"valid"}`, false}, {`{"zoomInId":3}`, false}, {`broken`, false}} {
		tree.Root.SetIALAttr("scroll", scroll.raw)
		rows, e := Infos(ctx, InfoRequest{IDs: ids, Batch: true, References: true, AV: true}, s)
		if e != nil || len(rows) != 2 || rows[0].ID != "block" || rows[0].RootID != "root" || rows[0].Name != "Title & value" || rows[0].Icon != "book" || rows[0].SubFileCount != 2 || rows[0].RefCount != 2 || rows[0].AttrViews[1].Name != "Database" {
			t.Fatal(rows, e)
		}
		_, kept := rows[0].IAL["scroll"]
		if kept != scroll.keep || tree.Root.IALAttr("scroll") != scroll.raw {
			t.Fatal("scroll mutated owner", rows[0].IAL)
		}
		rows[0].IAL["title"] = "changed"
		rows[0].RefIDs[0] = "changed"
		if rows[1].IAL["title"] == "changed" || rows[1].RefIDs[0] == "changed" {
			t.Fatal("aliased results")
		}
	}
	rows, e := Infos(ctx, InfoRequest{IDs: ids, Batch: true}, s)
	if e != nil || rows[0].RefIDs != nil || rows[0].AttrViews != nil || rows[0].RefCount != 0 {
		t.Fatal(rows, e)
	}
	rows, e = Infos(ctx, InfoRequest{Batch: true}, s)
	if e != nil || rows != nil {
		t.Fatal("empty batch shape", rows, e)
	}
	s.Subfiles = func(context.Context, *parse.Tree) (int, error) { return 0, errors.New("directory") }
	s.IgnoreSingleDirectoryError = true
	rows, e = Infos(ctx, InfoRequest{IDs: []string{"block"}}, s)
	if e != nil || len(rows) != 1 || rows[0].SubFileCount != 0 {
		t.Fatal(rows, e)
	}
	if rows, e = Infos(ctx, InfoRequest{IDs: ids, Batch: true}, s); e == nil || rows != nil {
		t.Fatal("batch directory failure lost", rows, e)
	}
}

func TestDocumentInfoFailuresAndOuterCancellation(t *testing.T) {
	for _, stage := range []string{"wait", "trees", "exists", "refs", "av", "fallback", "subfiles", "late-cancel"} {
		t.Run(stage, func(t *testing.T) {
			tree, s := infoFixture()
			tree.Root.SetIALAttr("scroll", `{"zoomInId":"valid"}`)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			boom := errors.New(stage)
			switch stage {
			case "wait":
				s.Wait = func(context.Context) error { return boom }
			case "trees":
				s.Trees = func(context.Context, []string) (map[string]*parse.Tree, error) { return nil, boom }
			case "exists":
				s.Exists = func(context.Context, string) (bool, error) { return false, boom }
			case "refs":
				s.References = func(context.Context, string, string) ([]string, error) { return nil, boom }
			case "av":
				s.AVName = func(context.Context, string) (string, error) { return "", boom }
			case "fallback":
				s.DefaultAVName = func(context.Context) (string, error) { return "", boom }
			case "subfiles":
				s.Subfiles = func(context.Context, *parse.Tree) (int, error) { return 0, boom }
			case "late-cancel":
				s.Subfiles = func(context.Context, *parse.Tree) (int, error) { cancel(); return 2, nil }
			}
			rows, e := Infos(ctx, InfoRequest{IDs: []string{"block"}}, s)
			if e == nil || rows != nil {
				t.Fatal(rows, e)
			}
		})
	}
	_, s := infoFixture()
	s.Trees = nil
	if _, e := Infos(context.Background(), InfoRequest{}, s); !errors.Is(e, ErrInfoDependency) {
		t.Fatal(e)
	}
}

func TestDocumentInfoReferencePromotionRetainsOriginalContainers(t *testing.T) {
	ctx := context.Background()
	defs := []*RefDefs{{RefID: "one"}, {RefID: "two"}, {RefID: "three"}}
	rows := map[string]indexdb.BlockRecord{"one": {ID: "one", ParentID: "list", Type: "p", Content: "ref", Markdown: "ref"}, "two": {ID: "two", ParentID: "quote", Type: "p", Content: "words"}, "three": {ID: "three", ParentID: "list", Type: "p", Content: "other", Markdown: "ordinary text"}, "list": {ID: "list", Type: "i", FContent: "other"}, "quote": {ID: "quote", Type: "b"}}
	s := RefPromotionServices{Blocks: func(_ context.Context, ids []string) (ret []indexdb.BlockRecord, e error) {
		for _, id := range ids {
			if b, ok := rows[id]; ok {
				ret = append(ret, b)
			}
		}
		return
	}, Parser: func(context.Context) (*lute.Lute, error) {
		return nativeparser.NewEditor(nativeparser.EditorOptions{}), nil
	}}
	got, e := PromoteRefDefs(ctx, defs, s)
	if e != nil || defs[0].RefID != "one" || defs[1].RefID != "quote" || defs[2].RefID != "list" || !reflect.DeepEqual(got, map[string]string{"list": "three", "quote": "two"}) {
		t.Fatal(defs, got, e)
	}
	rows["list"] = indexdb.BlockRecord{ID: "list", Type: "i", FContent: "different"}
	defs = []*RefDefs{{RefID: "three"}}
	if _, e = PromoteRefDefs(ctx, defs, s); e != nil || defs[0].RefID != "three" {
		t.Fatal("ordinary list paragraph promoted", defs, e)
	}
}
