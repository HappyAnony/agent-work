package blockview

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
)

func bookmarkServices() BookmarkServices {
	return BookmarkServices{
		Query: func(context.Context) ([]indexdb.BlockRecord, error) {
			return []indexdb.BlockRecord{
				{ID: "named", Type: "av", Name: "Named <alias>", IAL: `{: bookmark="z"}`, Markdown: `<div av-id="never-read"></div>`},
				{ID: "doc", Type: "d", Content: "SQL title", IAL: `{: bookmark="A"}`},
				{ID: "av", Type: "av", IAL: `{: bookmark="A"}`, Markdown: `<div av-id="database"></div>`},
				{ID: "body", Type: "p", Content: "SQL fallback", IAL: `{: bookmark="A"}`},
				{ID: "broad", Type: "p", Name: "Broad", IAL: `{: custom-bookmark="A"}`},
			}, nil
		},
		Parser: func(context.Context) (*lute.Lute, error) { return lute.New(), nil },
		Node: func(_ context.Context, id string) (*ast.Node, error) {
			if id == "doc" {
				n := &ast.Node{ID: id, Type: ast.NodeDocument}
				n.SetIALAttr("title", "Real document title")
				return n, nil
			}
			n := &ast.Node{ID: id, Type: ast.NodeParagraph}
			n.AppendChild(&ast.Node{Type: ast.NodeText, Tokens: []byte("Actual  <body>\n")})
			return n, nil
		},
		AttributeViewName: func(_ context.Context, id string) (string, error) {
			if id != "database" {
				return "", errors.New("name precedence broken")
			}
			return "AV title", nil
		},
	}
}
func TestBookmarkPresentationOriginalPriorityGroupingCountsAndCopies(t *testing.T) {
	s := bookmarkServices()
	rows, _ := s.Query(context.Background())
	s.Query = func(context.Context) ([]indexdb.BlockRecord, error) { return rows, nil }
	got, err := BookmarksView(context.Background(), false, s)
	if err != nil || len(got) != 3 {
		t.Fatal(got, err)
	}
	if got[0].Name != "" || got[1].Name != "A" || got[2].Name != "z" || got[1].Count != 3 || got[1].Depth != 0 || got[1].Type != "bookmark" {
		t.Fatal("groups", got)
	}
	want := []string{"Real document title", "AV title", "Actual&nbsp;&nbsp;&lt;body&gt;"}
	for i, b := range got[1].Blocks {
		if b.Content != want[i] || b.Depth != 1 {
			t.Fatal("original order/content/depth", i, b)
		}
	}
	if got[2].Blocks[0].Content != "Named &lt;alias&gt;" {
		t.Fatal("name did not precede AV", got[2].Blocks[0])
	}
	got[1].Blocks[0].IAL["bookmark"] = "caller mutation"
	got[1].Blocks[0].Content = "changed"
	again, err := BookmarksView(context.Background(), false, s)
	if err != nil || again[1].Blocks[0].IAL["bookmark"] != "A" || rows[1].Content != "SQL title" {
		t.Fatal("input/result alias", err)
	}
}
func TestBookmarkOutlinePreviewPreservesImageAndStyleWithOriginalOutput(t *testing.T) {
	engine := lute.New()
	makeNode := func() *ast.Node {
		tree := parse.Parse("", []byte("# Hello  ![alt](assets/a.png \"tooltip\")\n"), engine.ParseOptions)
		n := tree.Root.FirstChild
		n.SetIALAttr("style", "font-size: 30px;color: red;")
		return n
	}
	original := makeNode()
	var image *ast.Node
	ast.Walk(original, func(n *ast.Node, e bool) ast.WalkStatus {
		if e && n.Type == ast.NodeImage {
			image = n
		}
		return ast.WalkContinue
	})
	if image == nil {
		t.Fatal("missing image fixture")
	}
	title := image.ChildByType(ast.NodeLinkTitle)
	if title == nil {
		t.Fatal("missing tooltip")
	}
	style := original.IALAttr("style")
	want := Outline(makeNode(), engine)
	got, err := OutlinePreview(context.Background(), original, engine)
	if err != nil || got != want || strings.Contains(got, "tooltip") || !strings.Contains(got, "font-size: inherit;") {
		t.Fatal("original rendering mismatch", got, want, err)
	}
	if original.IALAttr("style") != style || image.ChildByType(ast.NodeLinkTitle) != title || title.Parent != image {
		t.Fatal("preview changed source AST")
	}
	for i := 0; i < 2; i++ {
		again, err := OutlinePreview(context.Background(), original, engine)
		if err != nil || again != got {
			t.Fatal("repeated preview changed", again, err)
		}
	}
	broken := lute.New()
	broken.RenderOptions = nil
	if text, err := OutlinePreview(context.Background(), original, broken); err == nil || text != "" {
		t.Fatal("bad renderer did not fail closed", text, err)
	}
	if original.IALAttr("style") != style || image.ChildByType(ast.NodeLinkTitle) != title || title.Parent != image {
		t.Fatal("failed preview changed source AST")
	}
	if text, err := OutlinePreview(context.Background(), nil, engine); err != nil || text != "" {
		t.Fatal(text, err)
	}
}
func TestBookmarkPresentationFailuresLateCancellationAndLegacyFallback(t *testing.T) {
	sentinel := errors.New("owner unavailable")
	for _, point := range []string{"query", "parser", "node", "av"} {
		for _, late := range []bool{false, true} {
			t.Run(point+map[bool]string{false: "/error", true: "/cancel"}[late], func(t *testing.T) {
				s := bookmarkServices()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				switch point {
				case "query":
					f := s.Query
					s.Query = func(ctx context.Context) ([]indexdb.BlockRecord, error) {
						if late {
							v, e := f(ctx)
							cancel()
							return v, e
						}
						return nil, sentinel
					}
				case "parser":
					f := s.Parser
					s.Parser = func(ctx context.Context) (*lute.Lute, error) {
						if late {
							v, e := f(ctx)
							cancel()
							return v, e
						}
						return nil, sentinel
					}
				case "node":
					f := s.Node
					s.Node = func(ctx context.Context, id string) (*ast.Node, error) {
						if late {
							v, e := f(ctx, id)
							cancel()
							return v, e
						}
						return nil, sentinel
					}
				case "av":
					f := s.AttributeViewName
					s.AttributeViewName = func(ctx context.Context, id string) (string, error) {
						if late {
							v, e := f(ctx, id)
							cancel()
							return v, e
						}
						return "", sentinel
					}
				}
				got, err := BookmarksView(ctx, false, s)
				want := sentinel
				if late {
					want = context.Canceled
				}
				if !errors.Is(err, want) || got != nil {
					t.Fatal("partial success on failure", got, err)
				}
			})
		}
	}
	s := bookmarkServices()
	s.Legacy = true
	warned := 0
	s.Warn = func(string, error) { warned++ }
	s.Node = func(context.Context, string) (*ast.Node, error) { return nil, sentinel }
	s.AttributeViewName = func(context.Context, string) (string, error) { return "partial legacy name", sentinel }
	got, err := BookmarksView(context.Background(), false, s)
	if err != nil || len(got) != 3 || warned != 2 || got[1].Blocks[0].Content != "SQL title" || got[1].Blocks[1].Content != "partial legacy name" || got[1].Blocks[2].Content != "SQL fallback" {
		t.Fatal("legacy error policy changed", got, warned, err)
	}
	s.Legacy = false
	s.Node = func(context.Context, string) (*ast.Node, error) { return nil, nil }
	if got, err = BookmarksView(context.Background(), false, s); err == nil || got != nil {
		t.Fatal("missing native node accepted")
	}
}
func TestBookmarkPresentationMissingDependenciesAndEmptyResult(t *testing.T) {
	for _, point := range []string{"ctx", "query", "parser", "node", "av", "nil-parser"} {
		s := bookmarkServices()
		ctx := context.Background()
		switch point {
		case "ctx":
			ctx = nil
		case "query":
			s.Query = nil
		case "parser":
			s.Parser = nil
		case "node":
			s.Node = nil
		case "av":
			s.AttributeViewName = nil
		case "nil-parser":
			s.Parser = func(context.Context) (*lute.Lute, error) { return nil, nil }
		}
		if _, err := BookmarksView(ctx, false, s); err == nil {
			t.Fatal("missing dependency accepted", point)
		}
	}
	s := bookmarkServices()
	s.Query = func(context.Context) ([]indexdb.BlockRecord, error) { return nil, nil }
	got, err := BookmarksView(context.Background(), false, s)
	if err != nil || !reflect.DeepEqual(got, Bookmarks{}) {
		t.Fatal("empty result must serialize as []", got, err)
	}
}
