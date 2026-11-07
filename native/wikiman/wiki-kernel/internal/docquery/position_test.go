package docquery

import (
	"context"
	"errors"
	"github.com/88250/lute"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"reflect"
	"strings"
	"testing"
	"wiki-kernel/internal/indexdb"
	"wiki-kernel/internal/nativeparser"
)

func positionFixture() (*parse.Tree, map[string]*ast.Node, PositionServices) {
	nodes := map[string]*ast.Node{}
	makeNode := func(id string, typ ast.NodeType, text string) *ast.Node {
		n := &ast.Node{ID: id, Type: typ}
		if text != "" {
			n.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte(text)})
		}
		nodes[id] = n
		return n
	}
	root := makeNode("root", ast.NodeDocument, "")
	root.SetIALAttr("title", "Unused title")
	h := makeNode("h1", ast.NodeHeading, "Heading & one")
	h.HeadingLevel = 1
	h.SetIALAttr("name", "Custom & heading")
	root.AppendChild(h)
	root.AppendChild(makeNode("p1", ast.NodeParagraph, "First"))
	list := makeNode("list", ast.NodeList, "")
	list.ListData = &ast.ListData{}
	item := makeNode("item", ast.NodeListItem, "")
	item.ListData = &ast.ListData{}
	item.AppendChild(makeNode("nested", ast.NodeParagraph, "List text"))
	list.AppendChild(item)
	root.AppendChild(list)
	sb := makeNode("super", ast.NodeSuperBlock, "")
	h2 := makeNode("h2", ast.NodeHeading, "Second heading")
	h2.HeadingLevel = 2
	sb.AppendChild(h2)
	sb.AppendChild(makeNode("inside", ast.NodeParagraph, "Inside"))
	root.AppendChild(sb)
	root.AppendChild(makeNode("after", ast.NodeParagraph, "After"))
	db := makeNode("av", ast.NodeAttributeView, "")
	db.AttributeViewID = "database"
	root.AppendChild(db)
	tree := &parse.Tree{Root: root, Box: "box", Path: "/root.docj"}
	s := PositionServices{Load: func(_ context.Context, id string) (*parse.Tree, error) {
		if id == "absent" {
			return nil, nil
		}
		return tree, nil
	}, Notebook: func(context.Context, string) (string, bool, error) { return "Notebook", true, nil }, RootPath: func(context.Context, string, string) (string, error) { return "/Indexed & title", nil }, AVName: func(context.Context, string) (string, error) { return "Database", nil }, Text: func(ctx context.Context, n *ast.Node, exclude []string, remove bool) (string, error) {
		return BlockText(ctx, n, exclude, remove, func(ctx context.Context, n *ast.Node, exclude []string) (string, error) {
			return indexdb.StaticContent(ctx, n, exclude, false, false, false, indexdb.ContentServices{})
		})
	}, Parser: func(context.Context) (*lute.Lute, error) {
		return nativeparser.NewEditor(nativeparser.EditorOptions{}), nil
	}}
	return tree, nodes, s
}

func TestBlockPositionOriginalIndexShapesAndFirstTree(t *testing.T) {
	ctx := context.Background()
	tree, n, s := positionFixture()
	calls := []string{}
	load := s.Load
	s.Load = func(ctx context.Context, id string) (*parse.Tree, error) {
		calls = append(calls, id)
		return load(ctx, id)
	}
	for _, tc := range []struct {
		id   string
		want int
	}{{"p1", 2}, {"nested", 3}, {"h2", 4}, {"inside", 4}, {"root", 6}, {"absent", 0}, {"unknown", 0}} {
		r, e := Position(ctx, PositionRequest{Mode: "index", ID: tc.id}, s)
		if e != nil || r.Index != tc.want {
			t.Fatal(tc, r, e)
		}
	}
	calls = nil
	ids := []string{"nested", "inside", "root", "outside", "nested"}
	r, e := Position(ctx, PositionRequest{Mode: "indexes", IDs: ids}, s)
	if e != nil || !reflect.DeepEqual(r.Indexes, map[string]int{"nested": 3, "inside": 4, "root": 0, "outside": 0}) || !reflect.DeepEqual(calls, []string{"nested"}) {
		t.Fatal(r, calls, e)
	}
	r, e = Position(ctx, PositionRequest{Mode: "indexes", IDs: []string{"absent", "nested"}}, s)
	if e != nil || len(r.Indexes) != 0 {
		t.Fatal(r, e)
	}
	calls = nil
	r, e = Position(ctx, PositionRequest{Mode: "indexes"}, s)
	if e != nil || r.Indexes == nil || len(calls) != 0 {
		t.Fatal(r, e, calls)
	}
	if n["nested"].Box != "" || n["nested"].Path != "" || tree.Root.Parent != nil {
		t.Fatal("index read annotated AST")
	}
}

func TestBlockBreadcrumbOriginalHeadingsContainersAndEmbed(t *testing.T) {
	ctx := context.Background()
	_, _, s := positionFixture()
	cases := []struct {
		id         string
		embed      bool
		mode       int
		ids, names []string
	}{
		{"after", false, 0, []string{"root", "h1", "h2", "after"}, []string{"Notebook/Indexed &amp; title", "Heading &amp; one", "Second heading", ""}},
		{"nested", false, 0, []string{"root", "h1", "item"}, []string{"Notebook/Indexed &amp; title", "Heading &amp; one", "List text"}},
		{"list", false, 0, []string{"root", "h1", "list"}, []string{"Notebook/Indexed &amp; title", "Heading &amp; one", ""}},
		{"h1", true, 2, []string{"root", "h1"}, []string{"Notebook/Indexed &amp; title", "Custom &amp; heading"}},
		{"h1", true, 0, []string{"root", "h1"}, []string{"Notebook/Indexed &amp; title", ""}},
		{"inside", true, 0, []string{"root", "h1", "h2", "inside"}, []string{"Notebook/Indexed &amp; title", "Heading &amp; one", "Second heading", ""}},
		{"root", false, 0, []string{"root"}, []string{""}},
		{"root", true, 0, []string{"root"}, []string{"Notebook/Indexed &amp; title"}},
	}
	for _, tc := range cases {
		r, e := Position(ctx, PositionRequest{Mode: "breadcrumb", ID: tc.id, Embed: tc.embed, HeadingMode: tc.mode}, s)
		if e != nil {
			t.Fatal(tc, e)
		}
		var ids, names []string
		for _, p := range r.Breadcrumb {
			ids = append(ids, p.ID)
			names = append(names, p.Name)
			if p.Children != nil {
				t.Fatal("children shape")
			}
		}
		if !reflect.DeepEqual(ids, tc.ids) || !reflect.DeepEqual(names, tc.names) {
			t.Fatal(tc, ids, names)
		}
	}
	r, e := Position(ctx, PositionRequest{Mode: "breadcrumb", ID: "after", ExcludeTypes: []string{"NodeText"}}, s)
	if e != nil || r.Breadcrumb[1].Name != "" || r.Breadcrumb[2].Name != "" {
		t.Fatal(r, e)
	}
	s.Notebook = func(context.Context, string) (string, bool, error) { return "", false, nil }
	r, e = Position(ctx, PositionRequest{Mode: "breadcrumb", ID: "after"}, s)
	if e != nil || r.Breadcrumb == nil || len(r.Breadcrumb) != 0 {
		t.Fatal(r, e)
	}
}

func TestBlockPositionFailuresAndLateCancellation(t *testing.T) {
	for _, stage := range []string{"load", "notebook", "path", "text", "av", "late-cancel"} {
		t.Run(stage, func(t *testing.T) {
			_, _, s := positionFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			boom := errors.New(stage)
			id := "after"
			switch stage {
			case "load":
				s.Load = func(context.Context, string) (*parse.Tree, error) { return nil, boom }
			case "notebook":
				s.Notebook = func(context.Context, string) (string, bool, error) { return "", false, boom }
			case "path":
				s.RootPath = func(context.Context, string, string) (string, error) { return "", boom }
			case "text":
				s.Text = func(context.Context, *ast.Node, []string, bool) (string, error) { return "", boom }
			case "av":
				id = "av"
				s.AVName = func(context.Context, string) (string, error) { return "", boom }
			case "late-cancel":
				s.RootPath = func(context.Context, string, string) (string, error) { cancel(); return "/partial", nil }
			}
			r, e := Position(ctx, PositionRequest{Mode: "breadcrumb", ID: id}, s)
			if e == nil || !reflect.DeepEqual(r, PositionResult{}) {
				t.Fatal(r, e)
			}
		})
	}
	if _, e := Position(context.Background(), PositionRequest{Mode: "indexes"}, PositionServices{}); !errors.Is(e, ErrPositionDependency) {
		t.Fatal(e)
	}
}

func TestBlockTextOriginalImageFallbackLengthAndDOMFirstBlock(t *testing.T) {
	ctx := context.Background()
	_, nodes, s := positionFixture()
	nodes["h1"].FirstChild.Tokens = []byte(strings.Repeat("长", 1100))
	r, e := Position(ctx, PositionRequest{Mode: "breadcrumb", ID: "after"}, s)
	if e != nil || len([]rune(r.Breadcrumb[1].Name)) != 1024 {
		t.Fatal(r, e)
	}
	n := &ast.Node{Type: ast.NodeParagraph}
	image := &ast.Node{Type: ast.NodeImage}
	image.AppendChild(&ast.Node{Type: ast.NodeLinkTitle, Tokens: []byte("Image title")})
	n.AppendChild(image)
	text, e := BlockText(ctx, n, nil, true, func(context.Context, *ast.Node, []string) (string, error) { return "", nil })
	if e != nil || text != "Image title" {
		t.Fatal(text, e)
	}
	r, e = Position(ctx, PositionRequest{Mode: "dom-text", Content: `<div data-type="NodeParagraph"><div contenteditable="true">First &amp; 世界</div></div><div data-type="NodeParagraph"><div contenteditable="true">Second</div></div>`}, s)
	if e != nil || r.Text != "First &amp; 世界" {
		t.Fatal(r, e)
	}
}

func TestBlockBreadcrumbTaskMarkersAndListHeadingSkip(t *testing.T) {
	ctx := context.Background()
	_, nodes, s := positionFixture()
	item := nodes["item"]
	item.ListData.Typ = 3
	marker := &ast.Node{Type: ast.NodeTaskListItemMarker}
	item.PrependChild(marker)
	r, e := Position(ctx, PositionRequest{Mode: "breadcrumb", ID: "nested"}, s)
	if e != nil || len(r.Breadcrumb) != 3 || r.Breadcrumb[2].ID != "item" || r.Breadcrumb[2].SubType != "t" {
		t.Fatal(r, e)
	}
	h := &ast.Node{Type: ast.NodeHeading, ID: "list-heading", HeadingLevel: 2}
	h.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("Hidden heading")})
	nodes["nested"].InsertBefore(h)
	r, e = Position(ctx, PositionRequest{Mode: "breadcrumb", ID: "nested"}, s)
	if e != nil || len(r.Breadcrumb) != 4 || r.Breadcrumb[3].ID != "nested" {
		t.Fatal(r, e)
	}
	for _, p := range r.Breadcrumb {
		if p.ID == "list-heading" {
			t.Fatal("list heading incorrectly promoted", r)
		}
	}
	r, e = Position(ctx, PositionRequest{Mode: "breadcrumb", ID: "nested", Embed: true}, s)
	if e != nil || r.Breadcrumb[len(r.Breadcrumb)-2].Name != "" {
		t.Fatal("embed ancestor container text", r, e)
	}
}
